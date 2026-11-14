// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/sidebar"
	"colonyhall/internal/modules/sidebar/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements sidebar.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// ListSections returns all active sections with their items for workspace and owner.
func (s *Store) ListSections(ctx context.Context, workspaceID, ownerID ids.ID) ([]sidebar.Section, error) {
	rows, err := s.q(ctx).ListSectionsByOwner(ctx, sqlcgen.ListSectionsByOwnerParams{
		WorkspaceID: workspaceID,
		OwnerID:     ownerID,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []sidebar.Section{}, nil
	}

	sectionIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		sectionIDs[i] = r.ID
	}
	itemRows, err := s.q(ctx).ListItemsBySectionIDs(ctx, sectionIDs)
	if err != nil {
		return nil, err
	}

	itemsBySection := make(map[uuid.UUID][]sidebar.Item)
	for _, it := range itemRows {
		itemsBySection[it.SectionID] = append(itemsBySection[it.SectionID], sidebar.Item{
			SectionID: it.SectionID,
			ChannelID: it.ChannelID,
			Position:  int(it.Position),
		})
	}

	result := make([]sidebar.Section, len(rows))
	for i, r := range rows {
		items := itemsBySection[r.ID]
		if items == nil {
			items = []sidebar.Item{}
		}
		result[i] = sidebar.Section{
			ID:          r.ID,
			WorkspaceID: r.WorkspaceID,
			OwnerID:     r.OwnerID,
			Name:        r.Name,
			Position:    int(r.Position),
			Collapsed:   r.Collapsed,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
			DeletedAt:   r.DeletedAt,
			Items:       items,
		}
	}
	return result, nil
}

// GetSection returns a section by ID with its items.
func (s *Store) GetSection(ctx context.Context, id ids.ID) (sidebar.Section, error) {
	r, err := s.q(ctx).GetSectionByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sidebar.Section{}, sidebar.ErrSectionNotFound
	}
	if err != nil {
		return sidebar.Section{}, err
	}
	itemRows, err := s.q(ctx).ListItemsBySectionID(ctx, id)
	if err != nil {
		return sidebar.Section{}, err
	}
	items := make([]sidebar.Item, len(itemRows))
	for i, it := range itemRows {
		items[i] = sidebar.Item{
			SectionID: it.SectionID,
			ChannelID: it.ChannelID,
			Position:  int(it.Position),
		}
	}
	return sidebar.Section{
		ID:          r.ID,
		WorkspaceID: r.WorkspaceID,
		OwnerID:     r.OwnerID,
		Name:        r.Name,
		Position:    int(r.Position),
		Collapsed:   r.Collapsed,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		DeletedAt:   r.DeletedAt,
		Items:       items,
	}, nil
}

// CreateSection creates a new sidebar section.
func (s *Store) CreateSection(ctx context.Context, sec sidebar.Section) error {
	return s.q(ctx).CreateSection(ctx, sqlcgen.CreateSectionParams{
		ID:          sec.ID,
		WorkspaceID: sec.WorkspaceID,
		OwnerID:     sec.OwnerID,
		Name:        sec.Name,
		Position:    int32(sec.Position),
		Collapsed:   sec.Collapsed,
		CreatedAt:   sec.CreatedAt,
		UpdatedAt:   sec.UpdatedAt,
	})
}

// UpdateSection updates a section's fields.
func (s *Store) UpdateSection(ctx context.Context, id ids.ID, name *string, position *int, collapsed *bool, updatedAt time.Time) (sidebar.Section, error) {
	var posPtr *int32
	if position != nil {
		p := int32(*position)
		posPtr = &p
	}
	row, err := s.q(ctx).UpdateSection(ctx, sqlcgen.UpdateSectionParams{
		ID:        id,
		Name:      name,
		Position:  posPtr,
		Collapsed: collapsed,
		UpdatedAt: updatedAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return sidebar.Section{}, sidebar.ErrSectionNotFound
	}
	if err != nil {
		return sidebar.Section{}, err
	}
	itemRows, err := s.q(ctx).ListItemsBySectionID(ctx, id)
	if err != nil {
		return sidebar.Section{}, err
	}
	items := make([]sidebar.Item, len(itemRows))
	for i, it := range itemRows {
		items[i] = sidebar.Item{
			SectionID: it.SectionID,
			ChannelID: it.ChannelID,
			Position:  int(it.Position),
		}
	}
	return sidebar.Section{
		ID:          row.ID,
		WorkspaceID: row.WorkspaceID,
		OwnerID:     row.OwnerID,
		Name:        row.Name,
		Position:    int(row.Position),
		Collapsed:   row.Collapsed,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		DeletedAt:   row.DeletedAt,
		Items:       items,
	}, nil
}

// DeleteSection soft-deletes a section and removes its items.
func (s *Store) DeleteSection(ctx context.Context, id ids.ID, deletedAt time.Time) error {
	if err := s.q(ctx).SoftDeleteSection(ctx, sqlcgen.SoftDeleteSectionParams{
		ID:        id,
		DeletedAt: &deletedAt,
	}); err != nil {
		return err
	}
	return s.q(ctx).DeleteItemsBySectionID(ctx, id)
}

// ReorderSections updates positions for multiple sections.
func (s *Store) ReorderSections(ctx context.Context, ownerID ids.ID, sectionIDs []ids.ID) error {
	for pos, id := range sectionIDs {
		if err := s.q(ctx).UpdateSectionPosition(ctx, sqlcgen.UpdateSectionPositionParams{
			ID:       id,
			Position: int32(pos),
			OwnerID:  ownerID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// SetItem puts a channel in a section, removing it from other sections of the owner.
func (s *Store) SetItem(ctx context.Context, ownerID, sectionID, channelID ids.ID, position int) error {
	if err := s.q(ctx).DeleteItemFromOwnerSections(ctx, sqlcgen.DeleteItemFromOwnerSectionsParams{
		ChannelID: channelID,
		OwnerID:   ownerID,
	}); err != nil {
		return err
	}
	return s.q(ctx).UpsertItem(ctx, sqlcgen.UpsertItemParams{
		SectionID: sectionID,
		ChannelID: channelID,
		Position:  int32(position),
	})
}

// RemoveItem removes a channel from a section.
func (s *Store) RemoveItem(ctx context.Context, sectionID, channelID ids.ID) error {
	return s.q(ctx).DeleteItem(ctx, sqlcgen.DeleteItemParams{
		SectionID: sectionID,
		ChannelID: channelID,
	})
}

// ReorderItems updates positions of channels in a section.
func (s *Store) ReorderItems(ctx context.Context, sectionID ids.ID, channelIDs []ids.ID) error {
	for pos, chID := range channelIDs {
		if err := s.q(ctx).UpdateItemPosition(ctx, sqlcgen.UpdateItemPositionParams{
			SectionID: sectionID,
			ChannelID: chID,
			Position:  int32(pos),
		}); err != nil {
			return err
		}
	}
	return nil
}
