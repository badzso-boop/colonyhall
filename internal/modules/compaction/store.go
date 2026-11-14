// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package compaction

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

type Store interface {
	// Settings
	GetSettings(ctx context.Context, workspaceID ids.ID) (Settings, error)
	UpsertSettings(ctx context.Context, st Settings) error

	// Channel override
	GetChannelThreshold(ctx context.Context, channelID ids.ID) (*int, error)
	SetChannelThreshold(ctx context.Context, channelID ids.ID, threshold *int) error

	// Unit counters
	GetUnit(ctx context.Context, channelID, threadRootID ids.ID) (CompactionUnit, error)
	UpsertUnit(ctx context.Context, u CompactionUnit) error
	AddTokensToUnit(ctx context.Context, workspaceID, channelID, threadRootID ids.ID, tokens, msgs int, now time.Time) (CompactionUnit, error)
	ResetUnit(ctx context.Context, channelID, threadRootID, compactionID ids.ID, remainingTokens, remainingMsgs int, now time.Time) error

	// Compactions
	InsertCompaction(ctx context.Context, c Compaction) error
	CompactionByID(ctx context.Context, id ids.ID) (Compaction, error)
	LatestDoneCompaction(ctx context.Context, channelID, threadRootID ids.ID) (Compaction, error)
	PendingCompaction(ctx context.Context, channelID, threadRootID ids.ID) (Compaction, error)
	FailPendingCompaction(ctx context.Context, id ids.ID, errorCode string, completedAt time.Time) error
	CompleteCompaction(ctx context.Context, id ids.ID, fileID, sysMsgID ids.ID, messagesCount, approxTokensIn, summaryTokens int, completedAt time.Time) error
	// CompactionCounts counts the running compactions and those failed since.
	CompactionCounts(ctx context.Context, workspaceID ids.ID, since time.Time) (running, failed int64, err error)
	CompactionIDsBySystemMessageIDs(ctx context.Context, msgIDs []ids.ID) (map[ids.ID]ids.ID, error)
}
