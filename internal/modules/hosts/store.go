package hosts

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists hosts, assignments and grants. Implementations: postgres,
// memory. Lookups return db.ErrNotFound when nothing matches.
type Store interface {
	CreatePairingCode(ctx context.Context, c PairingCode) error
	// UsePairingCode marks an unused, unexpired code as used and returns it.
	UsePairingCode(ctx context.Context, hash []byte, now time.Time) (PairingCode, error)

	CreateHost(ctx context.Context, h Host) error
	HostByID(ctx context.Context, id ids.ID) (Host, error)
	HostByTokenHash(ctx context.Context, hash []byte) (Host, error)
	ListHosts(ctx context.Context, workspaceID ids.ID) ([]Host, error)
	SetHostState(ctx context.Context, id ids.ID, state string) error
	SaveReport(ctx context.Context, id ids.ID, r Report, at time.Time) error
	Touch(ctx context.Context, id ids.ID, at time.Time) error

	// SetAssignment creates or replaces the agent's assignment.
	SetAssignment(ctx context.Context, a Assignment) error
	DeleteAssignment(ctx context.Context, agentID ids.ID) error
	AssignmentByAgent(ctx context.Context, agentID ids.ID) (Assignment, error)
	AssignmentsForHost(ctx context.Context, hostID ids.ID) ([]Assignment, error)

	AddGrant(ctx context.Context, g Grant) error
	GrantByID(ctx context.Context, id ids.ID) (Grant, error)
	RevokeGrant(ctx context.Context, id ids.ID, at time.Time) error
	// ActiveGrants lists non-revoked grants of the agent on the host, by path.
	ActiveGrants(ctx context.Context, hostID, agentID ids.ID) ([]Grant, error)
	// RevokeGrantsForAgent revokes every active grant of the agent.
	RevokeGrantsForAgent(ctx context.Context, agentID ids.ID, at time.Time) error
}
