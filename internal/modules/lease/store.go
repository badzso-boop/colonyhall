package lease

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store defines persistence operations for leases.
type Store interface {
	LockResource(ctx context.Context, workspaceID ids.ID, resource string) error
	GetActiveByResource(ctx context.Context, workspaceID ids.ID, resource string, now time.Time) (Lease, error)
	GetByID(ctx context.Context, id ids.ID) (Lease, error)
	Create(ctx context.Context, l Lease) error
	Extend(ctx context.Context, id ids.ID, newExpiresAt time.Time, note *string) error
	Release(ctx context.Context, id ids.ID, releasedBy ids.ID, now time.Time) error
	ListActive(ctx context.Context, workspaceID ids.ID, now time.Time) ([]Lease, error)
	ReleaseActiveByWorkspace(ctx context.Context, workspaceID ids.ID, releasedBy ids.ID, now time.Time) ([]Lease, error)
	ReleaseActiveByHolder(ctx context.Context, workspaceID ids.ID, holderID ids.ID, releasedBy ids.ID, now time.Time) ([]Lease, error)
}
