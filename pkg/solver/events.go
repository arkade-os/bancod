package solver

import (
	"context"
	"time"
)

// FulfillmentEvent describes a successful fulfillment of a banco offer.
// Amounts are in raw base units (sats for BTC, or the asset's own base unit).
type FulfillmentEvent struct {
	Pair          string
	DepositAsset  string
	DepositAmount uint64
	WantAsset     string
	WantAmount    uint64
	OfferTxid     string
	FulfillTxid   string
	Timestamp     time.Time
}

// FulfillmentListener is notified when the solver successfully fulfills an offer.
// Implementations should return quickly; any persistence or network I/O should
// be non-blocking or delegated to a background goroutine.
type FulfillmentListener interface {
	OnFulfill(ctx context.Context, evt FulfillmentEvent)
}
