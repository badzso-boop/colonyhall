// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package activity

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists the feed. Implementations: postgres, memory.
type Store interface {
	// Insert stores an item; an item for the same (recipient, source_seq,
	// kind) is skipped (a replayed event). It reports whether it was new.
	Insert(ctx context.Context, it Item) (bool, error)
	List(ctx context.Context, workspaceID, recipientID ids.ID, q Query) ([]Item, error)
	CountUnread(ctx context.Context, workspaceID, recipientID ids.ID) (int, error)
	// MarkRead marks the recipient's unread items read: the given ones, or
	// every one in a channel (channelID set), or all (both empty).
	MarkRead(ctx context.Context, recipientID ids.ID, itemIDs []ids.ID, channelID ids.ID, at time.Time) (int, error)
}
