// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package secrets

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Store persists secrets. Every call runs in the workspace's scope.
type Store interface {
	Insert(ctx context.Context, s Secret) error
	// Get returns a live secret of the workspace (db.ErrNotFound otherwise).
	Get(ctx context.Context, workspaceID, id ids.ID) (Secret, error)
	Replace(ctx context.Context, workspaceID, id ids.ID, sealed secretbox.Sealed, hint string, at time.Time) error
	Delete(ctx context.Context, workspaceID, id ids.ID, at time.Time) error
	// ListForRotation returns every live secret not sealed with the active
	// key, across workspaces (system scope: the rotate-secrets command).
	ListForRotation(ctx context.Context, activeKey string) ([]Secret, error)
	// Reseal stores the same value sealed with another key.
	Reseal(ctx context.Context, workspaceID, id ids.ID, sealed secretbox.Sealed) error
}
