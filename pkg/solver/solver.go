package solver

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/arkade-os/arkd/pkg/client-lib/client"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/sirupsen/logrus"

	"github.com/arkade-os/bancod/pkg/contract"
)

// Status represents the current state of the solver.
type Status struct {
	Running bool
}

// Solver watches the arkd transaction stream for banco swap offers
// and automatically fulfills matching offers based on configured pairs.
type Solver struct {
	Config

	mu     sync.RWMutex
	prices *priceCache

	ctx      context.Context
	cancelFn context.CancelFunc
	running  bool
}

// New creates a new Solver instance. Call Start to begin processing.
func New(cfg Config) *Solver {
	cfg = cfg.WithDefault()
	return &Solver{
		Config: cfg,
		prices: newPriceCache(cfg.PriceFeed, cfg.PriceCacheTTL),
	}
}

// Start begins monitoring the arkd transaction stream and processing offers.
func (s *Solver) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancelFn = cancel
	s.running = true

	go s.monitorStream(ctx)
}

// Stop stops the solver and cancels the monitoring goroutine.
func (s *Solver) Stop() {
	if s.cancelFn != nil {
		s.cancelFn()
		s.cancelFn = nil
		s.running = false
		s.Log.Info("solver stopped")
	}
}

// Status returns the current state of the solver.
func (s *Solver) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Running: s.running}
}

// monitorStream subscribes to the arkd transaction stream and processes
// ArkTx events.
func (s *Solver) monitorStream(ctx context.Context) {
	s.Log.Debug("starting transaction stream monitor")

	eventsCh, stop, err := s.SolverClient.Client().GetTransactionsStream(ctx)
	if err != nil {
		s.Log.WithError(err).Error("failed to establish connection to transaction stream")
		return
	}

	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			return
		case event, ok := <-eventsCh:
			if !ok {
				s.Log.Debug("transaction stream closed")
				return
			}
			if event.Err != nil {
				s.Log.WithError(event.Err).Error("error received from transaction stream")
				continue
			}
			if event.ArkTx == nil {
				continue
			}
			s.processArkTx(ctx, event.ArkTx)
		}
	}
}

// processArkTx handles a single ArkTx event from the transaction stream.
func (s *Solver) processArkTx(ctx context.Context, notification *client.TxNotification) {
	log := s.Log.WithField("txid", notification.Txid)

	tx, err := psbt.NewFromRawBytes(strings.NewReader(notification.Tx), true)
	if err != nil {
		log.WithError(err).Warn("failed to decode psbt")
		return
	}

	solverOffer, err := NewOffer(tx.UnsignedTx)
	if err != nil {
		log.WithError(err).Warn("failed to decode banco offer from extension")
		return
	}
	if solverOffer == nil {
		log.Debug("no banco offer or matching swap output in tx")
		return
	}
	pairs, err := s.PairsRepository.List(ctx)
	if err != nil {
		return
	}

	pair := findMatchingPair(pairs, solverOffer)
	if pair == nil {
		// no pair found for this offer, skip
		return
	}

	if solverOffer.WantAmount < pair.MinAmount || solverOffer.WantAmount > pair.MaxAmount {
		// amount not in range of pair config, skip
		return
	}

	feedPrice, err := s.prices.get(ctx, pair.PriceFeed)
	if err != nil {
		// get() returns the cached price alongside the error when only the
		// refresh failed. Use it if available, otherwise skip this offer.
		if feedPrice == 0 {
			s.Log.WithError(err).Warn("price feed unavailable, skipping offer")
			return
		}
		s.Log.WithError(err).Warn("using stale price feed")
	}
	if pair.InvertPrice {
		feedPrice = 1.0 / feedPrice
	}

	offerPrice, ok := solverOffer.ComputePrice(pair)
	if !ok {
		return
	}

	if !validatePrice(offerPrice, feedPrice) {
		return
	}

	// verify the balance
	if solverOffer.WantAsset == nil {
		balance, err := s.SolverClient.Balance(ctx)
		if err != nil {
			return
		}
		if balance.OffchainBalance.Total < solverOffer.WantAmount {
			return
		}
	}

	// lock before spending the contract in order to sync with other go routines processing different txs
	s.mu.Lock()
	result, err := contract.FulfillOffer(
		ctx,
		solverOffer.Offer,
		s.SolverClient,
		s.Introspector,
	)
	s.mu.Unlock()
	if err != nil {
		s.Log.WithError(err).Warn("fulfillment failed")
		return
	}

	s.Log.WithFields(logrus.Fields{
		"arkTxid": result.ArkTxid,
	}).Info("banco offer fulfilled successfully")

	// Notify listener outside the lock so DB I/O does not serialize fulfillment.
	if s.Listener != nil {
		s.Listener.OnFulfill(ctx, FulfillmentEvent{
			Pair:          pair.Pair,
			DepositAsset:  solverOffer.DepositAssetStr(),
			DepositAmount: solverOffer.DepositAmount,
			WantAsset:     solverOffer.WantAssetStr(),
			WantAmount:    solverOffer.WantAmount,
			OfferTxid:     solverOffer.FundingTxid,
			FulfillTxid:   result.ArkTxid,
			Timestamp:     time.Now().UTC(),
		})
	}
}

// findMatchingPair returns the first configured pair whose base and quote
// both match the offer.
func findMatchingPair(pairs []Pair, solverOffer *Offer) *Pair {
	depositAsset := solverOffer.DepositAssetStr()
	wantAsset := solverOffer.WantAssetStr()

	for i := range pairs {
		pair := &pairs[i]
		if pair.Base() != depositAsset {
			continue
		}
		if pair.Quote() != wantAsset {
			continue
		}
		return pair
	}
	return nil
}

// validatePrice makes sure the offer price is within 1% margin of the feedPrice
func validatePrice(offerPrice, feedPrice float64) bool {
	return offerPrice >= feedPrice*0.99 && offerPrice <= feedPrice*1.01
}
