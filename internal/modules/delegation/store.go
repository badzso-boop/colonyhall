package delegation

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists delegations. Implementations: postgres, memory.
type Store interface {
	Insert(ctx context.Context, d Delegation) error
	GetByID(ctx context.Context, id ids.ID) (Delegation, error)
	GetByQuestionID(ctx context.Context, questionID ids.ID) (Delegation, error)
	GetByMessageAndTarget(ctx context.Context, messageID, toAgentID ids.ID) (Delegation, error)
	UpdateStateConditional(ctx context.Context, id ids.ID, expectedState, newState string, decidedBy *ids.ID, decidedAt time.Time) (int64, error)
	ListByWorkspace(ctx context.Context, workspaceID ids.ID, state *string) ([]Delegation, error)
}
