package solver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/stretchr/testify/require"
)

// fakePlugin drives engine tests.
type fakePlugin struct {
	mu      sync.Mutex
	matchFn func(context.Context, *psbt.Packet) (any, bool)
	solveFn func(context.Context, any)
	matched int
	solved  []any
	solveWg sync.WaitGroup
}

func (f *fakePlugin) Match(ctx context.Context, tx *psbt.Packet) (any, bool) {
	f.mu.Lock()
	f.matched++
	f.mu.Unlock()
	if f.matchFn != nil {
		return f.matchFn(ctx, tx)
	}
	return nil, false
}

func (f *fakePlugin) Solve(ctx context.Context, intent any) {
	defer f.solveWg.Done()
	f.mu.Lock()
	f.solved = append(f.solved, intent)
	f.mu.Unlock()
	if f.solveFn != nil {
		f.solveFn(ctx, intent)
	}
}

// expectSolves marks how many Solve calls the test expects, so it can wait
// for the (concurrent) Solve goroutines to finish before asserting.
func (f *fakePlugin) expectSolves(n int) { f.solveWg.Add(n) }

func (f *fakePlugin) waitSolves(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() { f.solveWg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Solve goroutines did not finish within 1s")
	}
}

// runEngine spawns Run in a goroutine, returns done channel + result holder.
func runEngine(t *testing.T, s *Solver, ctx context.Context, ch <-chan *psbt.Packet) (chan struct{}, *error) {
	t.Helper()
	done := make(chan struct{})
	var runErr error
	go func() {
		defer close(done)
		runErr = s.Run(ctx, ch)
	}()
	return done, &runErr
}

func TestRun_ReturnsNilOnChannelClose(t *testing.T) {
	plugin := &fakePlugin{}
	s := New(plugin)
	ctx := context.Background()
	ch := make(chan *psbt.Packet)

	done, errp := runEngine(t, s, ctx, ch)
	close(ch)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s of channel close")
	}
	require.NoError(t, *errp)
}

func TestRun_ReturnsCtxErrOnCancel(t *testing.T) {
	plugin := &fakePlugin{}
	s := New(plugin)
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *psbt.Packet)

	done, errp := runEngine(t, s, ctx, ch)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s of ctx cancel")
	}
	require.ErrorIs(t, *errp, context.Canceled)
}

func TestRun_MatchOkFalseDoesNotCallSolve(t *testing.T) {
	plugin := &fakePlugin{
		matchFn: func(context.Context, *psbt.Packet) (any, bool) { return nil, false },
	}
	s := New(plugin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan *psbt.Packet, 1)
	ch <- &psbt.Packet{}
	close(ch)

	done, errp := runEngine(t, s, ctx, ch)
	<-done
	require.NoError(t, *errp)
	require.Empty(t, plugin.solved)
}

func TestRun_MatchOkTrueCallsSolve(t *testing.T) {
	plugin := &fakePlugin{
		matchFn: func(context.Context, *psbt.Packet) (any, bool) { return "intent-1", true },
	}
	plugin.expectSolves(1)
	s := New(plugin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan *psbt.Packet, 1)
	ch <- &psbt.Packet{}
	close(ch)

	done, errp := runEngine(t, s, ctx, ch)
	<-done
	plugin.waitSolves(t)
	require.NoError(t, *errp)
	require.Equal(t, []any{"intent-1"}, plugin.solved)
}

func TestRun_NilTxIsSkipped(t *testing.T) {
	plugin := &fakePlugin{}
	s := New(plugin)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan *psbt.Packet, 2)
	ch <- nil
	ch <- &psbt.Packet{}
	close(ch)

	done, errp := runEngine(t, s, ctx, ch)
	<-done
	require.NoError(t, *errp)
	// Only the non-nil tx should have been Matched.
	require.Equal(t, 1, plugin.matched)
}

func TestRun_MultiplePluginsAllSeeTx(t *testing.T) {
	p1 := &fakePlugin{
		matchFn: func(context.Context, *psbt.Packet) (any, bool) { return "p1", true },
	}
	p2 := &fakePlugin{
		matchFn: func(context.Context, *psbt.Packet) (any, bool) { return "p2", true },
	}
	p1.expectSolves(1)
	p2.expectSolves(1)
	s := New(p1, p2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan *psbt.Packet, 1)
	ch <- &psbt.Packet{}
	close(ch)

	done, errp := runEngine(t, s, ctx, ch)
	<-done
	p1.waitSolves(t)
	p2.waitSolves(t)
	require.NoError(t, *errp)
	require.Equal(t, []any{"p1"}, p1.solved)
	require.Equal(t, []any{"p2"}, p2.solved)
}
