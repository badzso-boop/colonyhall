package runs

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Store persists run errors. Implementations: postgres, memory. Lookups
// return db.ErrNotFound when nothing matches.
type Store interface {
	InsertError(ctx context.Context, e Error) error
	ErrorByID(ctx context.Context, id ids.ID) (Error, error)
}
