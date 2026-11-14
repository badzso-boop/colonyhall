// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"colonyhall/internal/modules/sidebar"
	"colonyhall/internal/platform/ids"
)

// Store implements sidebar.Store in memory.
type Store struct {
	mu       sync.RWMutex
	sections map[ids.ID]sidebar.Section
	items    map[ids.ID][]sidebar.Item // sectionID -> items
}

// New creates an empty memory sidebar store.
func New() *Store {
	return &Store{
		sections: make(map[ids.ID]sidebar.Section),
		items:    make(map[ids.ID][]sidebar.Item),
	}
}

// ListSections returns active sections for a workspace and owner.
func (s *Store) ListSections(ctx context.Context, workspaceID, ownerID ids.ID) ([]sidebar.Section, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []sidebar.Section
	for _, sec := range s.sections {
		if sec.WorkspaceID == workspaceID && sec.OwnerID == ownerID && sec.DeletedAt == nil {
			clone := sec
			items := s.items[sec.ID]
			clone.Items = make([]sidebar.Item, len(items))
			copy(clone.Items, items)
			slices.SortFunc(clone.Items, func(a, b sidebar.Item) int {
				return a.Position - b.Position
			})
			result = append(result, clone)
		}
	}
	slices.SortFunc(result, func(a, b sidebar.Section) int {
		if a.Position != b.Position {
			return a.Position - b.Position
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	})
	return result, nil
}

// GetSection returns a section by ID if not deleted.
func (s *Store) GetSection(ctx context.Context, id ids.ID) (sidebar.Section, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sec, ok := s.sections[id]
	if !ok || sec.DeletedAt != nil {
		return sidebar.Section{}, sidebar.ErrSectionNotFound
	}
	clone := sec
	items := s.items[sec.ID]
	clone.Items = make([]sidebar.Item, len(items))
	copy(clone.Items, items)
	slices.SortFunc(clone.Items, func(a, b sidebar.Item) int {
		return a.Position - b.Position
	})
	return clone, nil
}

// CreateSection inserts a new section.
func (s *Store) CreateSection(ctx context.Context, sec sidebar.Section) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sections[sec.ID] = sec
	s.items[sec.ID] = []sidebar.Item{}
	return nil
}

// UpdateSection updates a section's fields.
func (s *Store) UpdateSection(ctx context.Context, id ids.ID, name *string, position *int, collapsed *bool, updatedAt time.Time) (sidebar.Section, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sec, ok := s.sections[id]
	if !ok || sec.DeletedAt != nil {
		return sidebar.Section{}, sidebar.ErrSectionNotFound
	}
	if name != nil {
		sec.Name = *name
	}
	if position != nil {
		sec.Position = *position
	}
	if collapsed != nil {
		sec.Collapsed = *collapsed
	}
	sec.UpdatedAt = updatedAt
	s.sections[id] = sec

	clone := sec
	items := s.items[sec.ID]
	clone.Items = make([]sidebar.Item, len(items))
	copy(clone.Items, items)
	return clone, nil
}

// DeleteSection soft-deletes a section and clears its items.
func (s *Store) DeleteSection(ctx context.Context, id ids.ID, deletedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sec, ok := s.sections[id]
	if !ok || sec.DeletedAt != nil {
		return sidebar.ErrSectionNotFound
	}
	sec.DeletedAt = &deletedAt
	s.sections[id] = sec
	delete(s.items, id)
	return nil
}

// ReorderSections sets the positions of sections in order.
func (s *Store) ReorderSections(ctx context.Context, ownerID ids.ID, sectionIDs []ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for pos, id := range sectionIDs {
		if sec, ok := s.sections[id]; ok && sec.OwnerID == ownerID && sec.DeletedAt == nil {
			sec.Position = pos
			s.sections[id] = sec
		}
	}
	return nil
}

// SetItem assigns a channel to a section, removing it from other sections of the owner.
func (s *Store) SetItem(ctx context.Context, ownerID, sectionID, channelID ids.ID, position int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Remove from all sections owned by this user
	for sID, sec := range s.sections {
		if sec.OwnerID == ownerID {
			s.removeItemLocked(sID, channelID)
		}
	}

	// 2. Add/update in target section
	items := s.items[sectionID]
	idx := slices.IndexFunc(items, func(it sidebar.Item) bool {
		return it.ChannelID == channelID
	})
	if idx >= 0 {
		items[idx].Position = position
	} else {
		items = append(items, sidebar.Item{
			SectionID: sectionID,
			ChannelID: channelID,
			Position:  position,
		})
	}
	s.items[sectionID] = items
	return nil
}

// RemoveItem removes a channel from a section.
func (s *Store) RemoveItem(ctx context.Context, sectionID, channelID ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.removeItemLocked(sectionID, channelID)
	return nil
}

func (s *Store) removeItemLocked(sectionID, channelID ids.ID) {
	items := s.items[sectionID]
	filtered := make([]sidebar.Item, 0, len(items))
	for _, it := range items {
		if it.ChannelID != channelID {
			filtered = append(filtered, it)
		}
	}
	s.items[sectionID] = filtered
}

// ReorderItems updates positions of channels within a section.
func (s *Store) ReorderItems(ctx context.Context, sectionID ids.ID, channelIDs []ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	items := s.items[sectionID]
	for pos, chID := range channelIDs {
		for i := range items {
			if items[i].ChannelID == chID {
				items[i].Position = pos
			}
		}
	}
	s.items[sectionID] = items
	return nil
}
