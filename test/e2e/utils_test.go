package e2e_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	clientTypes "github.com/arkade-os/arkd/pkg/client-lib/types"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	arkdURL          = "localhost:7170"
	arkdHTTPURL      = "http://localhost:7171"
	introspectorAddr = "localhost:7173"
)

func runCommand(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func faucet(ctx context.Context, address string, amount float64) error {
	command := fmt.Sprintf("nigiri faucet %s %.8f", address, amount)
	_, err := runCommand(ctx, command)
	return err
}

func generateNoteCtx(_ context.Context, amount uint64) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	reqBody := bytes.NewReader([]byte(fmt.Sprintf(`{"amount": "%d"}`, amount)))
	req, err := http.NewRequest("POST", arkdHTTPURL+"/v1/admin/note", reqBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Basic YWRtaW46YWRtaW4=")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	// nolint:errcheck
	defer resp.Body.Close()
	var noteResp struct {
		Notes []string `json:"notes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&noteResp); err != nil {
		return "", err
	}
	if len(noteResp.Notes) == 0 {
		return "", fmt.Errorf("no notes returned from admin API")
	}
	return noteResp.Notes[0], nil
}

func generateNote(t *testing.T, amount uint64) string {
	t.Helper()
	note, err := generateNoteCtx(t.Context(), amount)
	require.NoError(t, err)
	return note
}

func faucetOffchain(t *testing.T, client arksdk.ArkClient, amount float64) clientTypes.Vtxo {
	t.Helper()
	offchainAddr, err := client.NewOffchainAddress(t.Context())
	require.NoError(t, err)
	note := generateNote(t, uint64(amount*1e8))
	wg := &sync.WaitGroup{}
	wg.Add(1)
	var incomingFunds []clientTypes.Vtxo
	var incomingErr error
	go func() {
		incomingFunds, incomingErr = client.NotifyIncomingFunds(t.Context(), offchainAddr)
		wg.Done()
	}()
	txid, err := client.RedeemNotes(t.Context(), []string{note})
	require.NoError(t, err)
	require.NotEmpty(t, txid)
	wg.Wait()
	require.NoError(t, incomingErr)
	require.NotEmpty(t, incomingFunds)
	time.Sleep(time.Second)
	return incomingFunds[0]
}

func setupArkClient(t *testing.T) arksdk.ArkClient {
	t.Helper()
	arkClient, err := arksdk.NewArkClient("")
	require.NoError(t, err)
	privkey, err := btcec.NewPrivateKey()
	require.NoError(t, err)
	err = arkClient.Init(t.Context(), arkdURL, hex.EncodeToString(privkey.Serialize()), "pass")
	require.NoError(t, err)
	err = arkClient.Unlock(t.Context(), "pass")
	require.NoError(t, err)
	syncCtx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	select {
	case ev, ok := <-arkClient.IsSynced(syncCtx):
		require.True(t, ok)
		require.NoError(t, ev.Err)
		require.True(t, ev.Synced)
	case <-syncCtx.Done():
		t.Fatal("timed out waiting for sync")
	}
	t.Cleanup(func() { arkClient.Stop() })
	return arkClient
}

func newIntroClient(t *testing.T) introclient.TransportClient {
	t.Helper()
	conn, err := grpc.NewClient(introspectorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	return introclient.NewGRPCClient(conn)
}

func issueAsset(t *testing.T, client arksdk.ArkClient, supply uint64) string {
	t.Helper()
	_, assetIds, err := client.IssueAsset(t.Context(), supply, nil, nil)
	require.NoError(t, err)
	require.Len(t, assetIds, 1)
	return assetIds[0].String()
}

func listVtxosWithAsset(t *testing.T, client arksdk.ArkClient, assetID string) []clientTypes.Vtxo {
	t.Helper()
	vtxos, _, err := client.ListVtxos(t.Context())
	require.NoError(t, err)
	var result []clientTypes.Vtxo
	for _, v := range vtxos {
		for _, a := range v.Assets {
			if a.AssetId == assetID {
				result = append(result, v)
				break
			}
		}
	}
	return result
}

// waitForCondition polls until fn returns true or timeout expires.
func waitForCondition(t *testing.T, timeout time.Duration, interval time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	t.Fatal("condition not met within timeout")
}
