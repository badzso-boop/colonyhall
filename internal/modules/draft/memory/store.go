// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory draft store.
package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"colonyhall/internal/modules/draft"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type key struct{ actor, channel, thread ids.ID }

// Store implements draft.Store in memory.
type Store struct {
	mu     sync.Mutex
	drafts map[key]draft.Draft
}

// New returns an empty store.
func New() *Store { return &Store{drafts: map[key]draft.Draft{}} }

func copyDraft(d draft.Draft) draft.Draft {
	d.FileIDs = slices.Clone(d.FileIDs)
	if d.FileIDs == nil {
		d.FileIDs = []ids.ID{}
	}
	d.Attachments = nil
	return d
}

// Upsert implements draft.Store.
func (s *Store) Upsert(_ context.Context, d draft.Draft) (draft.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key{d.ActorID, d.ChannelID, d.ThreadRootID}
	if old, ok := s.drafts[k]; ok {
		d.ID = old.ID
	}
	d = copyDraft(d)
	s.drafts[k] = d
	return copyDraft(d), nil
}

// Get implements draft.Store.
func (s *Store) Get(_ context.Context, actorID, channelID, threadRootID ids.ID) (draft.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.drafts[key{actorID, channelID, threadRootID}]
	if !ok {
		return draft.Draft{}, db.ErrNotFound
	}
	return copyDraft(d), nil
}

// Delete implements draft.Store.
func (s *Store) Delete(_ context.Context, actorID, channelID, threadRootID ids.ID, notAfter *time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key{actorID, channelID, threadRootID}
	d, ok := s.drafts[k]
	if !ok || (notAfter != nil && d.UpdatedAt.After(*notAfter)) {
		return false, nil
	}
	delete(s.drafts, k)
	return true, nil
}

// List implements draft.Store.
func (s *Store) List(_ context.Context, workspaceID, actorID ids.ID, limit int) ([]draft.Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []draft.Draft
	for _, d := range s.drafts {
		if d.WorkspaceID == workspaceID && d.ActorID == actorID {
			out = append(out, copyDraft(d))
		}
	}
	slices.SortFunc(out, func(a, b draft.Draft) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
