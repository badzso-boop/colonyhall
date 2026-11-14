// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of eventlog.Store.
package memory

import (
	"context"
	"sync"

	"colonyhall/internal/eventlog"
)

// Store keeps events in a slice. Safe for concurrent use.
type Store struct {
	mu      sync.RWMutex
	events  []eventlog.Event
	offsets map[string]int64
}

// New returns an empty store.
func New() *Store { return &Store{offsets: map[string]int64{}} }

// Append assigns the next sequence number and stores the event.
func (s *Store) Append(_ context.Context, e *eventlog.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.Seq = int64(len(s.events) + 1)
	s.events = append(s.events, *e)
	return nil
}

// Since returns events after the given sequence number.
func (s *Store) Since(_ context.Context, after int64, limit int) ([]eventlog.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if after < 0 {
		after = 0
	}
	if after >= int64(len(s.events)) {
		return nil, nil
	}
	end := after + int64(limit)
	if end > int64(len(s.events)) {
		end = int64(len(s.events))
	}
	out := make([]eventlog.Event, end-after)
	copy(out, s.events[after:end])
	return out, nil
}

// Head returns the newest sequence number.
func (s *Store) Head(context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.events)), nil
}

// Offset returns a consumer's stored position.
func (s *Store) Offset(_ context.Context, consumer string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.offsets[consumer], nil
}

// SetOffset stores a consumer's position.
func (s *Store) SetOffset(_ context.Context, consumer string, seq int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offsets[consumer] = seq
	return nil
}
