// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package dbstats

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store reads sizes from the database and keeps the daily samples.
// Implementations: postgres, memory (no database: Size and LargestTables
// return ErrNoDatabase).
type Store interface {
	// Size is the whole database's size in bytes.
	Size(ctx context.Context) (int64, error)
	// LargestTables lists the biggest tables first.
	LargestTables(ctx context.Context, limit int) ([]TableSize, error)
	// RecordSample stores the day's size unless that day already has one.
	RecordSample(ctx context.Context, s Sample) error
	// Samples lists the samples from since on, oldest first.
	Samples(ctx context.Context, since time.Time) ([]Sample, error)
	// PruneSamples deletes the samples before the day and returns how many went.
	PruneSamples(ctx context.Context, before time.Time) (int64, error)

	// Round 54 (memory: ErrNoDatabase).

	// WorkspaceIDs lists every workspace. System mode.
	WorkspaceIDs(ctx context.Context) ([]ids.ID, error)
	// WorkspaceRowBytes sums the workspace's rows in the large tables.
	WorkspaceRowBytes(ctx context.Context, workspaceID ids.ID) (RowBytes, error)
	// ChatTablesSize is the chat and search tables' size with indexes.
	ChatTablesSize(ctx context.Context) (int64, error)
}
