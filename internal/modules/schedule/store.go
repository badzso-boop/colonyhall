// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package schedule

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists schedules. Implementations: postgres, memory. Lookups
// return db.ErrNotFound when nothing matches.
type Store interface {
	Create(ctx context.Context, s Schedule) error
	ByID(ctx context.Context, id ids.ID) (Schedule, error)
	List(ctx context.Context, workspaceID ids.ID) ([]Schedule, error)
	// Due lists active schedules whose next run is at or before now.
	// Due lists the active schedules due at now, oldest first, except the
	// skipped workspaces' (round 51: suspended).
	Due(ctx context.Context, now time.Time, limit int, skip []ids.ID) ([]Schedule, error)
	// Claim moves an active schedule from the run it was due for to its
	// next run (or to done); false when someone else already did.
	Claim(ctx context.Context, id ids.ID, dueAt time.Time, next time.Time, state string, at time.Time) (bool, error)
	// Record stores the outcome of a run.
	Record(ctx context.Context, id ids.ID, messageID ids.ID, runErr string) error
	SetState(ctx context.Context, id ids.ID, state string) error
}
