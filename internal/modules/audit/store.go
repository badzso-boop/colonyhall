package audit

import (
	"context"

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
	// Forward returns entries with Seq > after, oldest first (for Verify).
	Forward(ctx context.Context, workspaceID ids.ID, after int64, limit int) ([]Entry, error)
}
