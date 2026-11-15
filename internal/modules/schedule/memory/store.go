// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of schedule.Store.
package memory

import (
	"bytes"
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements schedule.Store in memory.
type Store struct {
	mu  sync.Mutex
	all map[ids.ID]schedule.Schedule
}

// New returns an empty store.
func New() *Store { return &Store{all: map[ids.ID]schedule.Schedule{}} }

// Create stores a schedule.
func (s *Store) Create(_ context.Context, sc schedule.Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all[sc.ID] = sc
	return nil
}

// ByID loads a schedule.
func (s *Store) ByID(_ context.Context, id ids.ID) (schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.all[id]
	if !ok {
		return sc, db.ErrNotFound
	}
	return sc, nil
}

func sorted(list []schedule.Schedule) {
	sort.Slice(list, func(i, j int) bool {
		if !list[i].NextRunAt.Equal(list[j].NextRunAt) {
			return list[i].NextRunAt.Before(list[j].NextRunAt)
		}
		return bytes.Compare(list[i].ID[:], list[j].ID[:]) < 0
	})
}

// List lists a workspace's schedules by next run.
func (s *Store) List(_ context.Context, workspaceID ids.ID) ([]schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []schedule.Schedule
	for _, sc := range s.all {
		if sc.WorkspaceID == workspaceID {
			out = append(out, sc)
		}
	}
	sorted(out)
	return out, nil
}

// Due lists active schedules due at now.
func (s *Store) Due(_ context.Context, now time.Time, limit int, skip []ids.ID) ([]schedule.Schedule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []schedule.Schedule
	for _, sc := range s.all {
		if sc.State == schedule.StateActive && !sc.NextRunAt.After(now) && !slices.Contains(skip, sc.WorkspaceID) {
			out = append(out, sc)
		}
	}
	sorted(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Claim moves a due run on, once.
func (s *Store) Claim(_ context.Context, id ids.ID, dueAt, next time.Time, state string, at time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.all[id]
	if !ok || sc.State != schedule.StateActive || !sc.NextRunAt.Equal(dueAt) {
		return false, nil
	}
	sc.NextRunAt, sc.State, sc.LastRunAt = next, state, &at
	sc.RunCount++
	s.all[id] = sc
	return true, nil
}

// Record stores a run's outcome.
func (s *Store) Record(_ context.Context, id, messageID ids.ID, runErr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.all[id]
	if !ok {
		return db.ErrNotFound
	}
	sc.LastMessageID, sc.LastError = messageID, runErr
	s.all[id] = sc
	return nil
}

// SetState changes a schedule's state.
func (s *Store) SetState(_ context.Context, id ids.ID, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.all[id]
	if !ok {
		return db.ErrNotFound
	}
	sc.State = state
	s.all[id] = sc
	return nil
}
