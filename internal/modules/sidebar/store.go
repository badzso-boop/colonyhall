// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package sidebar

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store defines persistence for sidebar sections and items.
type Store interface {
	ListSections(ctx context.Context, workspaceID, ownerID ids.ID) ([]Section, error)
	GetSection(ctx context.Context, id ids.ID) (Section, error)
	CreateSection(ctx context.Context, s Section) error
	UpdateSection(ctx context.Context, id ids.ID, name *string, position *int, collapsed *bool, updatedAt time.Time) (Section, error)
	DeleteSection(ctx context.Context, id ids.ID, deletedAt time.Time) error
	ReorderSections(ctx context.Context, ownerID ids.ID, sectionIDs []ids.ID) error
	SetItem(ctx context.Context, ownerID, sectionID, channelID ids.ID, position int) error
	RemoveItem(ctx context.Context, sectionID, channelID ids.ID) error
	ReorderItems(ctx context.Context, sectionID ids.ID, channelIDs []ids.ID) error
}
