// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

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
