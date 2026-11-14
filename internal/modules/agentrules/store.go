// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package agentrules

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Store persists shared agent rules versions. Implementations: postgres, memory.
// Lookups return db.ErrNotFound when nothing matches.
type Store interface {
	Insert(ctx context.Context, r Rules) error
	GetLatest(ctx context.Context, workspaceID ids.ID) (Rules, error)
	GetVersion(ctx context.Context, workspaceID ids.ID, version int) (Rules, error)
	ListHistory(ctx context.Context, workspaceID ids.ID) ([]RulesMeta, error)
}
