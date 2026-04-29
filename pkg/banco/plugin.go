package banco

import (
	"context"
	"fmt"
	"time"

	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/sirupsen/logrus"

	"github.com/arkade-os/bancod/pkg/contract"
	"github.com/arkade-os/bancod/pkg/solver"
)

// MatchedOffer is the typed intent emitted by Plugin.Match and consumed
// by Plugin.Solve. It carries the parsed offer plus the matched pair so
// Solve doesn't redo lookups.
type MatchedOffer struct {
	Offer *Offer
	Pair  *Pair
}

// Plugin implements solver.Plugin[*MatchedOffer] for the banco swap protocol.
type Plugin struct {
	arkClient    arksdk.ArkClient
	introspector introclient.TransportClient
	pairs        PairRepository
	prices       *priceCache
	listener     FulfillmentListener
	log          logrus.FieldLogger
}

// NewPlugin builds a banco Plugin from a Config.
func NewPlugin(cfg Config) *Plugin {
	cfg = cfg.WithDefault()
	return &Plugin{
		arkClient:    cfg.SolverClient,
		introspector: cfg.Introspector,
		pairs:        cfg.PairsRepository,
		prices:       newPriceCache(cfg.PriceFeed, cfg.PriceCacheTTL),
		listener:     cfg.Listener,
		log:          cfg.Log,
	}
}

var _ solver.Plugin = (*Plugin)(nil)

// Match decodes the banco offer from the tx extension, looks up a matching
// configured pair, validates amount range and price, and returns a typed
// MatchedOffer if everything checks out.
func (p *Plugin) Match(ctx context.Context, tx *psbt.Packet) (any, bool) {
	if tx == nil {
		return nil, false
	}

	// 1. Decode banco offer from extension.
	offer, err := NewOffer(tx.UnsignedTx)
	if err != nil {
		p.log.WithError(err).Warn("failed to decode banco offer")
		return nil, false
	}
	if offer == nil {
		return nil, false
	}

	// 2. Look up a matching pair.
	if p.pairs == nil {
		return nil, false
	}
	pairs, err := p.pairs.List(ctx)
	if err != nil {
		p.log.WithError(err).Warn("failed to list pairs")
		return nil, false
	}
	pair := findMatchingPair(pairs, offer)
	if pair == nil {
		return nil, false
	}

	// 3. Range-check WantAmount.
	if offer.WantAmount < pair.MinAmount || offer.WantAmount > pair.MaxAmount {
		return nil, false
	}

	// 4. Validate price within 1% of feed.
	feedPrice, err := p.prices.get(ctx, pair.PriceFeed)
	if err != nil && feedPrice == 0 {
		p.log.WithError(err).Warn("price feed unavailable, skipping offer")
		return nil, false
	}
	if err != nil {
		p.log.WithError(err).Warn("using stale price feed")
	}
	if pair.InvertPrice {
		feedPrice = 1.0 / feedPrice
	}

	offerPrice, ok := offer.ComputePrice(pair)
	if !ok {
		return nil, false
	}
	if !validatePrice(offerPrice, feedPrice) {
		return nil, false
	}

	return &MatchedOffer{Offer: offer, Pair: pair}, true
}

// Solve fulfills the matched offer atomically using contract.FulfillOffer
// and notifies the FulfillmentListener if one is configured.
func (p *Plugin) Solve(ctx context.Context, intent any) {
	m, ok := intent.(*MatchedOffer)
	if !ok || m == nil || m.Offer == nil {
		return
	}

	// BTC deposits require sufficient offchain balance.
	if m.Offer.WantAsset == nil {
		bal, err := p.arkClient.Balance(ctx)
		if err != nil {
			p.log.WithError(err).Warn("failed to get balance")
			return
		}
		if bal.OffchainBalance.Total < m.Offer.WantAmount {
			err := fmt.Errorf(
				"insufficient offchain balance: have %d want %d",
				bal.OffchainBalance.Total, m.Offer.WantAmount,
			)
			p.log.Warn(err.Error())
			return
		}
	}

	result, err := contract.FulfillOffer(ctx, m.Offer.Offer, p.arkClient, p.introspector)
	if err != nil {
		p.log.WithError(err).Warn("fulfillment failed")
		return
	}

	if p.listener == nil {
		return
	}

	p.listener.OnFulfill(ctx, FulfillmentEvent{
		Pair:          m.Pair.Pair,
		DepositAsset:  m.Offer.DepositAssetStr(),
		DepositAmount: m.Offer.DepositAmount,
		WantAsset:     m.Offer.WantAssetStr(),
		WantAmount:    m.Offer.WantAmount,
		OfferTxid:     m.Offer.FundingTxid,
		FulfillTxid:   result.ArkTxid,
		Timestamp:     time.Now().UTC(),
	})
}

// findMatchingPair returns the first pair whose base+quote both match
// the offer's deposit and want assets.
func findMatchingPair(pairs []Pair, o *Offer) *Pair {
	depositAsset := o.DepositAssetStr()
	wantAsset := o.WantAssetStr()
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
