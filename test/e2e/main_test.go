//go:build e2e

package e2e_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcec/v2"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/arkade-os/bancod/internal/core/application"
	"github.com/arkade-os/bancod/internal/core/ports"
	sqlitedb "github.com/arkade-os/bancod/internal/infrastructure/db/sqlite"
	"github.com/arkade-os/bancod/pkg/banco"
	"github.com/arkade-os/bancod/pkg/solver"
)

// mockPriceFeed always returns a fixed price of 1.0.
// This makes any offer with roughly 1:1 ratio pass the 1% margin check.
type mockPriceFeed struct{}

func (m *mockPriceFeed) Fetch(_ context.Context, _ string) (float64, error) {
	return 1.0, nil
}

var (
	takerSvc    *application.TakerService
	pairRepo    ports.PairRepository
	takerClient arksdk.ArkClient
)

func TestMain(m *testing.M) {
	log.SetLevel(log.DebugLevel)
	ctx := context.Background()

	if err := refillArkd(ctx); err != nil {
		log.Fatalf("failed to refill arkd: %s", err)
	}

	// Create taker's ArkClient
	var err error
	takerClient, err = setupTakerClient(ctx)
	if err != nil {
		log.Fatalf("failed to setup taker client: %s", err)
	}

	// Fund taker with offchain BTC
	if err := fundTaker(ctx, takerClient); err != nil {
		log.Fatalf("failed to fund taker: %s", err)
	}

	// SQLite pair repo in temp dir
	tmpDir, err := os.MkdirTemp("", "bancod-e2e-*")
	if err != nil {
		log.Fatalf("failed to create temp dir: %s", err)
	}
	// nolint:errcheck
	defer os.RemoveAll(tmpDir)

	db, err := sqlitedb.OpenDB(tmpDir)
	if err != nil {
		log.Fatalf("failed to open db: %s", err)
	}
	// nolint:errcheck
	defer db.Close()

	pairRepo = sqlitedb.NewPairRepository(db)
	tradeRepo := sqlitedb.NewTradeRepository(db)

	// Introspector client
	introConn, err := grpc.NewClient(introspectorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to introspector: %s", err)
	}
	introClient := introclient.NewGRPCClient(introConn)

	// Build solver
	plugin := banco.NewPlugin(banco.Config{
		SolverClient:    takerClient,
		Introspector:    introClient,
		PairsRepository: pairRepo,
		PriceFeed:       &mockPriceFeed{},
		Log:             log.StandardLogger(),
	})
	s := solver.New(plugin)

	takerSvc = application.NewTakerService(s, pairRepo, tradeRepo, takerClient, takerClient.Indexer(), log.StandardLogger())
	takerSvc.Start()
	defer takerSvc.Stop()

	os.Exit(m.Run())
}

func setupTakerClient(ctx context.Context) (arksdk.ArkClient, error) {
	client, err := arksdk.NewArkClient("")
	if err != nil {
		return nil, err
	}
	privkey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	if err := client.Init(ctx, arkdURL, hex.EncodeToString(privkey.Serialize()), "pass"); err != nil {
		return nil, err
	}
	if err := client.Unlock(ctx, "pass"); err != nil {
		return nil, err
	}
	syncCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case ev, ok := <-client.IsSynced(syncCtx):
		if !ok || !ev.Synced {
			return nil, fmt.Errorf("taker client failed to sync")
		}
	case <-syncCtx.Done():
		return nil, fmt.Errorf("taker client sync timed out")
	}
	return client, nil
}

func fundTaker(ctx context.Context, client arksdk.ArkClient) error {
	bal, err := client.Balance(ctx)
	if err != nil {
		return err
	}
	if bal.OffchainBalance.Total >= 100000 {
		return nil // already funded
	}

	// Use the offchain note-based funding (same as faucetOffchain but without *testing.T).
	offchainAddr, err := client.NewOffchainAddress(ctx)
	if err != nil {
		return fmt.Errorf("failed to get offchain address: %w", err)
	}

	note, err := generateNoteCtx(ctx, 200000) // 200k sats
	if err != nil {
		return fmt.Errorf("failed to generate note: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := client.NotifyIncomingFunds(ctx, offchainAddr)
		done <- err
	}()

	if _, err := client.RedeemNotes(ctx, []string{note}); err != nil {
		return fmt.Errorf("failed to redeem note: %w", err)
	}

	if err := <-done; err != nil {
		return fmt.Errorf("notify incoming funds failed: %w", err)
	}

	time.Sleep(2 * time.Second)
	return nil
}

func refillArkd(ctx context.Context) error {
	arkdExec := "docker exec bancod-arkd arkd"
	command := fmt.Sprintf("%s wallet balance", arkdExec)
	out, err := runCommand(ctx, command)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`available:\s*([0-9]+\.[0-9]+)`)
	matches := re.FindStringSubmatch(out)
	if len(matches) < 2 {
		return fmt.Errorf("could not parse arkd balance from: %s", out)
	}
	balance, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return err
	}
	if delta := 5.0 - balance; delta >= 1 {
		addrCmd := fmt.Sprintf("%s wallet address", arkdExec)
		address, err := runCommand(ctx, addrCmd)
		if err != nil {
			return err
		}
		for range int(delta) {
			if err := faucet(ctx, strings.TrimSpace(address), 1); err != nil {
				return err
			}
		}
	}
	time.Sleep(5 * time.Second)
	return nil
}
