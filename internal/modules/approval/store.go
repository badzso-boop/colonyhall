package approval

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists approval requests.
type Store interface {
	Create(ctx context.Context, r ApprovalRequest) error
	ByID(ctx context.Context, id ids.ID) (ApprovalRequest, error)
	Decide(ctx context.Context, id ids.ID, state string, decidedBy ids.ID, message string, decidedAt time.Time) error
	ExpirePendingByWorkspace(ctx context.Context, workspaceID ids.ID, decidedAt time.Time) (int64, error)
	ListPendingByWorkspace(ctx context.Context, workspaceID ids.ID) ([]ApprovalRequest, error)
}
