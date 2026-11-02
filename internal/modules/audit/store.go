package audit

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists audit entries. Implementations: postgres, memory.
type Store interface {
	// Insert stores e unless an entry with the same Seq exists (replay-safe).
	Insert(ctx context.Context, e Entry) error
	// Last returns the newest entry of the workspace, or db.ErrNotFound.
	Last(ctx context.Context, workspaceID ids.ID) (Entry, error)
	// Page returns entries with Seq < before (0 = newest), newest first.
	Page(ctx context.Context, workspaceID ids.ID, before int64, limit int) ([]Entry, error)
	// Search is Page with a filter.
	Search(ctx context.Context, workspaceID ids.ID, f Filter) ([]Entry, error)
	// Count counts the entries a filter selects.
	Count(ctx context.Context, workspaceID ids.ID, f Filter) (int64, error)
	// Actions lists the workspace's distinct actions, sorted.
	Actions(ctx context.Context, workspaceID ids.ID) ([]string, error)
	// Forward returns entries with Seq > after, oldest first (for Verify).
	Forward(ctx context.Context, workspaceID ids.ID, after int64, limit int) ([]Entry, error)
}

// Filter selects audit entries; zero values do not filter.
type Filter struct {
	ActorID    ids.ID
	Actions    []string
	TargetType string
	From       *time.Time
	To         *time.Time
	Before     int64
	Limit      int
}
