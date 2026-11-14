// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package draft

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists drafts. Implementations: postgres, memory.
type Store interface {
	// Upsert stores the draft for (actor, channel, thread), replacing the
	// one there; it keeps the existing row's ID.
	Upsert(ctx context.Context, d Draft) (Draft, error)
	// Get returns the actor's draft in one place, or db.ErrNotFound.
	Get(ctx context.Context, actorID, channelID, threadRootID ids.ID) (Draft, error)
	// Delete removes the actor's draft in one place if it was last saved
	// at or before notAfter (nil: whatever its time); it reports whether a
	// row went.
	Delete(ctx context.Context, actorID, channelID, threadRootID ids.ID, notAfter *time.Time) (bool, error)
	// List returns the actor's drafts, newest first.
	List(ctx context.Context, workspaceID, actorID ids.ID, limit int) ([]Draft, error)
}
