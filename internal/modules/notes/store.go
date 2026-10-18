package notes

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Store defines persistence operations for agent notes.
type Store interface {
	Create(ctx context.Context, note Note) (Note, error)
	GetLatest(ctx context.Context, workspaceID, agentID, threadRootID ids.ID) (Note, error)
	ListByAgent(ctx context.Context, workspaceID, agentID ids.ID, threadRootID *ids.ID) ([]Note, error)
}
