package runs

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists run errors, runs, and daily usage. Implementations: postgres, memory.
// Lookups return db.ErrNotFound when nothing matches.
type Store interface {
	InsertError(ctx context.Context, e Error) error
	ErrorByID(ctx context.Context, id ids.ID) (Error, error)

	InsertRun(ctx context.Context, r Run) error
	ListRuns(ctx context.Context, workspaceID ids.ID, agentID ids.ID, limit int) ([]Run, error)
	Usage(ctx context.Context, workspaceID ids.ID, since time.Time) ([]DayUsage, error)
	TokensSince(ctx context.Context, workspaceID, agentID ids.ID, since time.Time) (int64, error)
}
