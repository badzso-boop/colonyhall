package memory

import (
	"context"
	"sort"
	"sync"

	"colonyhall/internal/modules/agentrules"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type key struct {
	workspaceID ids.ID
	version     int
}

// Store is an in-memory implementation of agentrules.Store.
type Store struct {
	mu    sync.RWMutex
	rules map[key]agentrules.Rules
}

// New returns a new empty in-memory agentrules Store.
func New() *Store {
	return &Store{
		rules: make(map[key]agentrules.Rules),
	}
}

// Insert stores a new agent rules version.
func (s *Store) Insert(_ context.Context, r agentrules.Rules) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key{workspaceID: r.WorkspaceID, version: r.Version}
	if _, exists := s.rules[k]; exists {
		return db.ErrConflict
	}
	s.rules[k] = r
	return nil
}

// GetLatest returns the highest version of rules for the workspace.
func (s *Store) GetLatest(_ context.Context, workspaceID ids.ID) (agentrules.Rules, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest agentrules.Rules
	found := false
	for k, r := range s.rules {
		if k.workspaceID == workspaceID {
			if !found || r.Version > latest.Version {
				latest = r
				found = true
			}
		}
	}
	if !found {
		return agentrules.Rules{}, db.ErrNotFound
	}
	return latest, nil
}

// GetVersion returns a specific version of rules for the workspace.
func (s *Store) GetVersion(_ context.Context, workspaceID ids.ID, version int) (agentrules.Rules, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.rules[key{workspaceID: workspaceID, version: version}]
	if !ok {
		return agentrules.Rules{}, db.ErrNotFound
	}
	return r, nil
}

// ListHistory returns version history for the workspace ordered by version descending.
func (s *Store) ListHistory(_ context.Context, workspaceID ids.ID) ([]agentrules.RulesMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var meta []agentrules.RulesMeta
	for k, r := range s.rules {
		if k.workspaceID == workspaceID {
			meta = append(meta, agentrules.RulesMeta{
				Version:   r.Version,
				CreatedBy: r.CreatedBy,
				CreatedAt: r.CreatedAt,
			})
		}
	}
	sort.Slice(meta, func(i, j int) bool {
		return meta[i].Version > meta[j].Version
	})
	return meta, nil
}
