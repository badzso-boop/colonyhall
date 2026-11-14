// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory saved-item store.
package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"colonyhall/internal/modules/saved"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements saved.Store in memory.
type Store struct {
	mu    sync.Mutex
	items map[ids.ID]saved.Item
}

// New returns an empty store.
func New() *Store { return &Store{items: map[ids.ID]saved.Item{}} }

// Insert implements saved.Store.
func (s *Store) Insert(_ context.Context, it saved.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it.MessageID != ids.Nil {
		for _, o := range s.items {
			if o.ActorID == it.ActorID && o.MessageID == it.MessageID {
				return db.ErrConflict
			}
		}
	}
	s.items[it.ID] = it
	return nil
}

// Update implements saved.Store.
func (s *Store) Update(_ context.Context, it saved.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.items[it.ID]
	if !ok {
		return nil
	}
	old.Note, old.RemindAt, old.RemindedAt, old.DoneAt = it.Note, it.RemindAt, it.RemindedAt, it.DoneAt
	s.items[it.ID] = old
	return nil
}

// Get implements saved.Store.
func (s *Store) Get(_ context.Context, id ids.ID) (saved.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok {
		return saved.Item{}, db.ErrNotFound
	}
	return it, nil
}

// ByMessage implements saved.Store.
func (s *Store) ByMessage(_ context.Context, actorID, messageID ids.ID) (saved.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.items {
		if it.ActorID == actorID && it.MessageID == messageID && messageID != ids.Nil {
			return it, nil
		}
	}
	return saved.Item{}, db.ErrNotFound
}

// Delete implements saved.Store.
func (s *Store) Delete(_ context.Context, id ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
	return nil
}

func newestFirst(a, b saved.Item) int {
	if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
		return c
	}
	return bytes.Compare(b.ID[:], a.ID[:])
}

// List implements saved.Store.
func (s *Store) List(_ context.Context, workspaceID, actorID ids.ID, f saved.Filter, after *saved.Cursor, limit int) ([]saved.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []saved.Item
	for _, it := range s.items {
		if it.WorkspaceID != workspaceID || it.ActorID != actorID {
			continue
		}
		if after != nil && newestFirst(saved.Item{CreatedAt: after.At, ID: after.ID}, it) >= 0 {
			continue // not older than the cursor
		}
		switch f {
		case saved.FilterReminders:
			if it.RemindAt == nil || it.DoneAt != nil {
				continue
			}
		case saved.FilterDone:
			if it.DoneAt == nil {
				continue
			}
		}
		out = append(out, it)
	}
	slices.SortFunc(out, newestFirst)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Count implements saved.Store.
func (s *Store) Count(_ context.Context, workspaceID, actorID ids.ID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, it := range s.items {
		if it.WorkspaceID == workspaceID && it.ActorID == actorID {
			n++
		}
	}
	return n, nil
}

func due(it saved.Item, now time.Time) bool {
	return it.RemindAt != nil && !it.RemindAt.After(now) && it.RemindedAt == nil && it.DoneAt == nil
}

// Due implements saved.Store.
func (s *Store) Due(_ context.Context, now time.Time, limit int) ([]saved.Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []saved.Item
	for _, it := range s.items {
		if due(it, now) {
			out = append(out, it)
		}
	}
	slices.SortFunc(out, func(a, b saved.Item) int { return a.RemindAt.Compare(*b.RemindAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ClaimReminder implements saved.Store.
func (s *Store) ClaimReminder(_ context.Context, id ids.ID, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[id]
	if !ok || !due(it, at) {
		return false, nil
	}
	it.RemindedAt = &at
	s.items[id] = it
	return true, nil
}
