// Package memory is the in-memory reference implementation of identity.Store.
package memory

import (
	"context"
	"sort"
	"sync"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements identity.Store in memory.
type Store struct {
	mu         sync.RWMutex
	workspaces map[ids.ID]identity.Workspace
	actors     map[ids.ID]identity.Actor
	tokens     map[string]identity.Token // key: string(hash)
}

// New returns an empty store.
func New() *Store {
	return &Store{
		workspaces: map[ids.ID]identity.Workspace{},
		actors:     map[ids.ID]identity.Actor{},
		tokens:     map[string]identity.Token{},
	}
}

// CreateWorkspace stores a workspace.
func (s *Store) CreateWorkspace(_ context.Context, w identity.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workspaces[w.ID] = w
	return nil
}

// CreateActor stores an actor, rejecting duplicate handles.
func (s *Store) CreateActor(_ context.Context, a identity.Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.actors {
		if other.WorkspaceID == a.WorkspaceID && other.Handle == a.Handle {
			return db.ErrConflict
		}
	}
	s.actors[a.ID] = a
	return nil
}

// ActorByID loads one actor.
func (s *Store) ActorByID(_ context.Context, id ids.ID) (identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.actors[id]
	if !ok {
		return identity.Actor{}, db.ErrNotFound
	}
	return a, nil
}

// ActorByHandle loads one actor by @handle.
func (s *Store) ActorByHandle(_ context.Context, workspaceID ids.ID, handle string) (identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, a := range s.actors {
		if a.WorkspaceID == workspaceID && a.Handle == handle {
			return a, nil
		}
	}
	return identity.Actor{}, db.ErrNotFound
}

// ActorsByIDs loads several actors, sorted by handle.
func (s *Store) ActorsByIDs(_ context.Context, idList []ids.ID) ([]identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []identity.Actor
	for _, id := range idList {
		if a, ok := s.actors[id]; ok {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out, nil
}

// CreateToken stores a token.
func (s *Store) CreateToken(_ context.Context, t identity.Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[string(t.Hash)] = t
	return nil
}

// ActorByTokenHash finds the token's actor.
func (s *Store) ActorByTokenHash(_ context.Context, hash []byte) (identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[string(hash)]
	if !ok {
		return identity.Actor{}, db.ErrNotFound
	}
	a, ok := s.actors[t.ActorID]
	if !ok {
		return identity.Actor{}, db.ErrNotFound
	}
	return a, nil
}

// ListActors lists every actor in the workspace, sorted by handle.
func (s *Store) ListActors(_ context.Context, workspaceID ids.ID) ([]identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []identity.Actor
	for _, a := range s.actors {
		if a.WorkspaceID == workspaceID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Handle < out[j].Handle })
	return out, nil
}
