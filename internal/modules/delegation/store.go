package delegation

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists delegations, grants, and settings. Implementations: postgres, memory.
type Store interface {
	Insert(ctx context.Context, d Delegation) error
	GetByID(ctx context.Context, id ids.ID) (Delegation, error)
	GetByQuestionID(ctx context.Context, questionID ids.ID) (Delegation, error)
	GetByMessageAndTarget(ctx context.Context, messageID, toAgentID ids.ID) (Delegation, error)
	UpdateStateConditional(ctx context.Context, id ids.ID, expectedState, newState string, decidedBy *ids.ID, decidedAt time.Time) (int64, error)
	// ApproveViaGrant moves a pending delegation to approved, marked as covered by a grant.
	ApproveViaGrant(ctx context.Context, id ids.ID, decidedBy ids.ID, decidedAt time.Time) (int64, error)
	ListByWorkspace(ctx context.Context, workspaceID ids.ID, state *string) ([]Delegation, error)
	ListPendingByPairAndConversation(ctx context.Context, workspaceID, agentA, agentB, channelID ids.ID, threadRootID ids.ID) ([]Delegation, error)

	// Grants
	InsertGrant(ctx context.Context, g Grant) error
	GetGrantByID(ctx context.Context, id ids.ID) (Grant, error)
	GetActiveGrant(ctx context.Context, workspaceID, agentA, agentB, channelID ids.ID, threadRootID ids.ID, now time.Time) (Grant, error)
	UpdateGrantLastUsed(ctx context.Context, id ids.ID, lastUsedAt time.Time) error
	RevokeGrantConditional(ctx context.Context, id ids.ID, revokedBy ids.ID, revokedAt time.Time) (int64, error)
	ListGrants(ctx context.Context, workspaceID ids.ID, activeOnly bool, now time.Time) ([]Grant, error)

	// Settings
	GetSettings(ctx context.Context, workspaceID ids.ID) (Settings, error)
	UpsertSettings(ctx context.Context, s Settings) error
}
