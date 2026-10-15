// Package memory is the in-memory reference implementation of runs.Store.
package memory

import (
	"context"
	"sync"

	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements runs.Store in memory.
type Store struct {
	mu     sync.RWMutex
	errors map[ids.ID]runs.Error
}

// New returns an empty store.
func New() *Store { return &Store{errors: map[ids.ID]runs.Error{}} }

// InsertError stores an error.
func (s *Store) InsertError(_ context.Context, e runs.Error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors[e.ID] = e
	return nil
}

// ErrorByID loads an error.
func (s *Store) ErrorByID(_ context.Context, id ids.ID) (runs.Error, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.errors[id]
	if !ok {
		return e, db.ErrNotFound
	}
	return e, nil
}
