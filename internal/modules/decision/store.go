package decision

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Store defines persistence operations for decisions.
type Store interface {
	Insert(ctx context.Context, d Decision) error
	NextNumber(ctx context.Context, workspaceID ids.ID) (int, error)
	GetByNumber(ctx context.Context, workspaceID ids.ID, number int) (Decision, error)
	List(ctx context.Context, workspaceID ids.ID, limit int, query string, validOnly bool) ([]Decision, error)
}
