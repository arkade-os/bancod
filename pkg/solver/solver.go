package solver

import (
	"context"
	"runtime/debug"
	"sync"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/sirupsen/logrus"
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
	log     logrus.FieldLogger
}

// New wraps Plugins in a Solver runtime.
func New(plugins ...Plugin) *Solver {
	return &Solver{plugins: plugins, log: logrus.StandardLogger()}
}

// WithLogger returns a copy of s using the given logger for panic reports.
// Pass nil to disable panic logging entirely (panics are still recovered).
func (s *Solver) WithLogger(log logrus.FieldLogger) *Solver {
	cp := *s
	cp.log = log
	return &cp
}

// Run consumes txs sequentially and dispatches Match -> (Solve if ok).
// Solves run concurrently; Run waits for all in-flight Solves before
// returning, so callers get clean shutdown semantics. Panics inside
// Match or Solve are recovered and logged so one buggy plugin can't
// crash the bot.
//
//   - returns ctx.Err() when ctx is canceled (after in-flight Solves drain)
//   - returns nil when the txs channel is closed (after in-flight Solves drain)
func (s *Solver) Run(ctx context.Context, txs <-chan *psbt.Packet) error {
	var wg sync.WaitGroup
	defer wg.Wait()

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
				intent, matched := s.safeMatch(ctx, p, tx)
				if !matched {
					continue
				}
				wg.Add(1)
				go func(p Plugin, intent any) {
					defer wg.Done()
					s.safeSolve(ctx, p, intent)
				}(p, intent)
			}
		}
	}
}

// safeMatch runs p.Match with panic recovery.
func (s *Solver) safeMatch(ctx context.Context, p Plugin, tx *psbt.Packet) (intent any, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s.logPanic(r, "plugin Match panicked")
			intent, ok = nil, false
		}
	}()
	return p.Match(ctx, tx)
}

// safeSolve runs p.Solve with panic recovery.
func (s *Solver) safeSolve(ctx context.Context, p Plugin, intent any) {
	defer func() {
		if r := recover(); r != nil {
			s.logPanic(r, "plugin Solve panicked")
		}
	}()
	p.Solve(ctx, intent)
}

// logPanic emits a panic report unless the solver was configured without a logger.
func (s *Solver) logPanic(r any, msg string) {
	if s.log == nil {
		return
	}
	s.log.WithField("panic", r).WithField("stack", string(debug.Stack())).Error(msg)
}
