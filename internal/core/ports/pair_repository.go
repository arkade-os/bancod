package ports

import (
	"context"
	"errors"

	"github.com/arkade-os/bancod/pkg/solver"
)

// ErrPairNotFound is returned by PairRepository.Update when the pair does
// not exist. Callers should map this to a NotFound API error.
var ErrPairNotFound = errors.New("pair not found")

// PairRepository extends solver.PairRepository with CRUD operations
// for managing trading pairs. The embedded solver.PairRepository provides
// the read-only List method used by the solver engine.
type PairRepository interface {
	solver.PairRepository // List(ctx context.Context) ([]solver.Pair, error)
	Add(ctx context.Context, pair solver.Pair) error
	Update(ctx context.Context, pair solver.Pair) error
	Remove(ctx context.Context, pairName string) error
}
