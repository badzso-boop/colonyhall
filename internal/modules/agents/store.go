package agents

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Store keeps the agents' own instructions. Implementations: postgres,
// memory. InstructionsByAgent returns db.ErrNotFound when there are none.
type Store interface {
	CreateInstructions(ctx context.Context, in Instructions) error
	// InstructionsByAgent returns the newest version.
	InstructionsByAgent(ctx context.Context, agentID ids.ID) (Instructions, error)
}
