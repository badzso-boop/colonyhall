// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of audit.Store.
package memory

import (
	"context"
	"slices"
	"sort"
	"sync"

	"colonyhall/internal/modules/audit"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements audit.Store in memory.
type Store struct {
	mu      sync.RWMutex
	entries map[int64]audit.Entry
}

// New returns an empty store.
func New() *Store { return &Store{entries: map[int64]audit.Entry{}} }

// Insert stores an entry once per seq.
func (s *Store) Insert(_ context.Context, e audit.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[e.Seq]; !ok {
		s.entries[e.Seq] = e
	}
	return nil
}

func (s *Store) sorted(workspaceID ids.ID) []audit.Entry {
	var out []audit.Entry
	for _, e := range s.entries {
		if e.WorkspaceID == workspaceID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out
}

// Last returns the newest entry of the workspace.
func (s *Store) Last(_ context.Context, workspaceID ids.ID) (audit.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.sorted(workspaceID)
	if len(all) == 0 {
		return audit.Entry{}, db.ErrNotFound
	}
	return all[len(all)-1], nil
}

// Page returns entries before a seq, newest first.
func (s *Store) Page(_ context.Context, workspaceID ids.ID, before int64, limit int) ([]audit.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.sorted(workspaceID)
	var out []audit.Entry
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		if before == 0 || all[i].Seq < before {
			out = append(out, all[i])
		}
	}
	return out, nil
}

// Forward returns entries after a seq, oldest first.
func (s *Store) Forward(_ context.Context, workspaceID ids.ID, after int64, limit int) ([]audit.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []audit.Entry
	for _, e := range s.sorted(workspaceID) {
		if e.Seq > after && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

// Tamper changes a stored entry's payload; tests use it to prove that
// Verify detects edits.
func (s *Store) Tamper(seq int64, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[seq]
	e.Payload = payload
	s.entries[seq] = e
}

func selected(e audit.Entry, f audit.Filter) bool {
	switch {
	case f.ActorID != ids.Nil && e.ActorID != f.ActorID,
		len(f.Actions) > 0 && !slices.Contains(f.Actions, e.Action),
		f.TargetType != "" && e.TargetType != f.TargetType,
		f.From != nil && e.At.Before(*f.From),
		f.To != nil && !e.At.Before(*f.To):
		return false
	}
	return true
}

// Search is Page with a filter.
func (s *Store) Search(_ context.Context, workspaceID ids.ID, f audit.Filter) ([]audit.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.sorted(workspaceID)
	var out []audit.Entry
	for i := len(all) - 1; i >= 0 && len(out) < f.Limit; i-- {
		if (f.Before == 0 || all[i].Seq < f.Before) && selected(all[i], f) {
			out = append(out, all[i])
		}
	}
	return out, nil
}

// Count counts the entries a filter selects.
func (s *Store) Count(_ context.Context, workspaceID ids.ID, f audit.Filter) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var n int64
	for _, e := range s.sorted(workspaceID) {
		if selected(e, f) {
			n++
		}
	}
	return n, nil
}

// Actions lists the workspace's distinct actions, sorted.
func (s *Store) Actions(_ context.Context, workspaceID ids.ID) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	out := []string{}
	for _, e := range s.sorted(workspaceID) {
		if !seen[e.Action] {
			seen[e.Action] = true
			out = append(out, e.Action)
		}
	}
	sort.Strings(out)
	return out, nil
}
