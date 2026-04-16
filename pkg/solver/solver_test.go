package solver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// validatePrice tests
// ---------------------------------------------------------------------------

func TestValidatePrice(t *testing.T) {
	tests := []struct {
		name       string
		offerPrice float64
		feedPrice  float64
		want       bool
	}{
		{
			name:       "exact match within 1% margin",
			offerPrice: 100.0,
			feedPrice:  100.0,
			want:       true,
		},
		{
			name:       "at lower bound (99% of feed)",
			offerPrice: 99.0,
			feedPrice:  100.0,
			want:       true,
		},
		{
			name:       "at upper bound (101% of feed)",
			offerPrice: 101.0,
			feedPrice:  100.0,
			want:       true,
		},
		{
			name:       "below lower bound",
			offerPrice: 98.9,
			feedPrice:  100.0,
			want:       false,
		},
		{
			name:       "above upper bound",
			offerPrice: 101.1,
			feedPrice:  100.0,
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validatePrice(tc.offerPrice, tc.feedPrice)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// findMatchingPair tests
// ---------------------------------------------------------------------------

func TestFindMatchingPair_Match(t *testing.T) {
	// BTC deposit, want = "" (BTC) -> pair "BTC/"
	offer := &Offer{DepositAsset: nil}
	// offer.WantAsset is nil, so WantAssetStr() == ""

	pairs := []Pair{
		{Pair: "BTC/"},    // Base="BTC", Quote=""
		{Pair: "OTHER/X"}, // won't match
	}

	result := findMatchingPair(pairs, offer)
	require.NotNil(t, result)
	assert.Equal(t, "BTC/", result.Pair)
}

func TestFindMatchingPair_NoMatchWrongBase(t *testing.T) {
	assetId := testAssetId(t)
	offer := &Offer{DepositAsset: assetId}

	pairs := []Pair{
		{Pair: "BTC/"}, // Base="BTC", but offer deposits an asset
	}

	result := findMatchingPair(pairs, offer)
	assert.Nil(t, result)
}

func TestFindMatchingPair_NoMatchWrongQuote(t *testing.T) {
	assetId := testAssetId(t)
	offer := &Offer{DepositAsset: nil} // BTC deposit
	offer.WantAsset = assetId          // wants an asset, so WantAssetStr() != ""

	pairs := []Pair{
		{Pair: "BTC/"}, // Quote="" but offer wants an asset
	}

	result := findMatchingPair(pairs, offer)
	assert.Nil(t, result)
}

func TestFindMatchingPair_EmptyPairs(t *testing.T) {
	offer := &Offer{DepositAsset: nil}
	result := findMatchingPair([]Pair{}, offer)
	assert.Nil(t, result)
}

func TestFindMatchingPair_AssetDepositAssetWant(t *testing.T) {
	depositAsset := testAssetId(t)
	wantAsset := testAssetId(t)

	offer := &Offer{DepositAsset: depositAsset}
	offer.WantAsset = wantAsset

	depositStr := offer.DepositAssetStr()
	wantStr := offer.WantAssetStr()

	pairs := []Pair{
		{Pair: depositStr + "/" + wantStr},
	}

	result := findMatchingPair(pairs, offer)
	require.NotNil(t, result)
	assert.Equal(t, depositStr+"/"+wantStr, result.Pair)
}

// ---------------------------------------------------------------------------
// New() tests
// ---------------------------------------------------------------------------

func TestNew_ReturnsNonNil(t *testing.T) {
	cfg := Config{}
	s := New(cfg)
	require.NotNil(t, s)
}

func TestNew_AppliesDefaults(t *testing.T) {
	cfg := Config{}
	s := New(cfg)
	require.NotNil(t, s)

	assert.Equal(t, defaultPriceCacheTTL, s.PriceCacheTTL, "default TTL should be applied")
	require.NotNil(t, s.Log, "default logger should be set")
}

// ---------------------------------------------------------------------------
// Status / lifecycle tests (no real clients needed)
// ---------------------------------------------------------------------------

func TestSolver_Status_InitiallyNotRunning(t *testing.T) {
	s := New(Config{})
	status := s.Status()
	assert.False(t, status.Running, "solver should not be running before Start()")
}

func TestSolver_Status_AfterManualFlag(t *testing.T) {
	s := New(Config{})

	// Directly set the flag to simulate a running state without calling Start()
	// (avoids needing real ARK client connections)
	s.running = true
	status := s.Status()
	assert.True(t, status.Running)

	s.running = false
	status = s.Status()
	assert.False(t, status.Running)
}

func TestSolver_Stop_NilCancelFn(t *testing.T) {
	// Stop() on a solver that was never started should not panic
	s := New(Config{})
	assert.NotPanics(t, func() {
		s.Stop()
	})
}
