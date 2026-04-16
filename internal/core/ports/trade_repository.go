package ports

import (
	"context"
	"time"
)

// Trade is a persisted record of a single offer fulfillment.
// Amounts are in raw base units (sats for BTC, asset base units otherwise).
type Trade struct {
	ID            int64
	Pair          string
	DepositAsset  string
	DepositAmount uint64
	WantAsset     string
	WantAmount    uint64
	OfferTxid     string
	FulfillTxid   string
	CreatedAt     time.Time
}

// TradeRepository persists fulfillments and exposes them for the dashboard.
type TradeRepository interface {
	Add(ctx context.Context, trade Trade) error
	List(ctx context.Context, limit int) ([]Trade, error)
}
