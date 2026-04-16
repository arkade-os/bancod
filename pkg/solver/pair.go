package solver

import (
	"context"
	"strings"
)

// Pair defines a trading pair and its constraints for the solver.
// The Pair field uses the format "{base}/{quote}" where each side is either
// "BTC" (for native bitcoin) or the hex asset ID (for arkade assets).
// Examples: "a1b2c3.../BTC", "BTC/d4e5f6...", "a1b2c3.../d4e5f6..."
type Pair struct {
	Pair          string `json:"pair"`          // e.g. "a1b2c3.../BTC"
	MinAmount     uint64 `json:"minAmount"`     // satoshis
	MaxAmount     uint64 `json:"maxAmount"`     // satoshis
	BaseDecimals  int    `json:"baseDecimals"`  // decimal precision of the base asset
	QuoteDecimals int    `json:"quoteDecimals"` // decimal precision of the quote asset
	PriceFeed     string `json:"priceFeed"`     // price API URL
	InvertPrice   bool   `json:"invertPrice"`   // if true, use 1/feedPrice for comparison
}

// Base returns the base asset of the pair (e.g. "BTC" from "BTC/USDT").
func (p Pair) Base() string {
	parts := strings.SplitN(p.Pair, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// Quote returns the quote asset of the pair (e.g. "USDT" from "BTC/USDT").
func (p Pair) Quote() string {
	parts := strings.SplitN(p.Pair, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

type PairRepository interface {
	List(ctx context.Context) ([]Pair, error)
}
