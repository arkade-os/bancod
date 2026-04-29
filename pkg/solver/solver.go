package solver

import (
	"context"

	"github.com/btcsuite/btcd/btcutil/psbt"
)

// Plugin is the protocol-specific contract a Solver runs.
type Plugin interface {
	// Match decodes/filters tx.
	//   ok=false  -> ignore tx (not for us)
	//   ok=true   -> hand intent to Solve
	Match(ctx context.Context, tx *psbt.Packet) (intent any, ok bool)

	// Solve reacts to a matched intent.
	Solve(ctx context.Context, intent any)
}

// Solver is the runtime that drives Plugins against a tx stream.
type Solver struct {
	plugins []Plugin
}

// New wraps Plugins in a Solver runtime.
func New(plugins ...Plugin) *Solver {
	return &Solver{plugins: plugins}
}

// Run consumes txs sequentially and dispatches Match -> (Solve if ok).
//
//   - returns ctx.Err() when ctx is canceled
//   - returns nil when the txs channel is closed
func (s *Solver) Run(ctx context.Context, txs <-chan *psbt.Packet) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case tx, ok := <-txs:
			if !ok {
				return nil
			}
			if tx == nil {
				continue
			}
			for _, p := range s.plugins {
				intent, ok := p.Match(ctx, tx)
				if !ok {
					continue
				}
				go p.Solve(ctx, intent)
			}
		}
	}
}
