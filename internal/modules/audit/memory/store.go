// Package memory is the in-memory reference implementation of audit.Store.
package memory

import (
	"context"
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
