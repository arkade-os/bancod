package application

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/arkade-os/bancod/internal/core/ports"
	"github.com/arkade-os/bancod/pkg/solver"
)

// persistTimeout caps how long the listener waits for a single trade insert.
// We detach from the caller's context (the solver tx-stream context) so
// shutdown does not abort persistence of an already-fulfilled offer.
const persistTimeout = 5 * time.Second

// tradeListener adapts solver.FulfillmentListener to ports.TradeRepository,
// persisting each fulfillment as a Trade row.
type tradeListener struct {
	repo ports.TradeRepository
	log  logrus.FieldLogger
}

// NewTradeListener returns a solver.FulfillmentListener that writes fulfillments
// to the given TradeRepository.
func NewTradeListener(repo ports.TradeRepository, log logrus.FieldLogger) solver.FulfillmentListener {
	if log == nil {
		log = logrus.New()
	}
	return &tradeListener{repo: repo, log: log}
}

func (l *tradeListener) OnFulfill(_ context.Context, evt solver.FulfillmentEvent) {
	trade := ports.Trade{
		Pair:          evt.Pair,
		DepositAsset:  evt.DepositAsset,
		DepositAmount: evt.DepositAmount,
		WantAsset:     evt.WantAsset,
		WantAmount:    evt.WantAmount,
		OfferTxid:     evt.OfferTxid,
		FulfillTxid:   evt.FulfillTxid,
		CreatedAt:     evt.Timestamp,
	}
	ctx, cancel := context.WithTimeout(context.Background(), persistTimeout)
	defer cancel()
	if err := l.repo.Add(ctx, trade); err != nil {
		l.log.WithError(err).WithField("fulfillTxid", evt.FulfillTxid).
			Error("failed to persist trade")
	}
}
