// Package memory is the in-memory reference implementation of identity.Store.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

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
	tokenByID  map[ids.ID]string         // token id → hash key
	creds      map[ids.ID]string
	sessions   map[string]identity.Session // key: string(token hash)
	revoked    map[ids.ID]bool
}

// New returns an empty store.
func New() *Store {
	return &Store{
		workspaces: map[ids.ID]identity.Workspace{},
		actors:     map[ids.ID]identity.Actor{},
		tokens:     map[string]identity.Token{},
		tokenByID:  map[ids.ID]string{},
		creds:      map[ids.ID]string{},
		sessions:   map[string]identity.Session{},
		revoked:    map[ids.ID]bool{},
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
	s.tokenByID[t.ID] = string(t.Hash)
	return nil
}

// ActorByTokenHash finds the token's actor.
func (s *Store) ActorByTokenHash(_ context.Context, hash []byte) (identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[string(hash)]
	if !ok || t.RevokedAt != nil {
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

// ActorsByHandle finds the handle in every workspace.
func (s *Store) ActorsByHandle(_ context.Context, handle string) ([]identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []identity.Actor
	for _, a := range s.actors {
		if a.Handle == handle {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkspaceID.String() < out[j].WorkspaceID.String() })
	return out, nil
}

// WorkspaceByName loads a workspace.
func (s *Store) WorkspaceByName(_ context.Context, name string) (identity.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, w := range s.workspaces {
		if w.Name == name {
			return w, nil
		}
	}
	return identity.Workspace{}, db.ErrNotFound
}

// Credential returns the actor's password hash.
func (s *Store) Credential(_ context.Context, actorID ids.ID) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.creds[actorID]
	if !ok {
		return "", db.ErrNotFound
	}
	return h, nil
}

// SetCredential stores or replaces the actor's password hash.
func (s *Store) SetCredential(_ context.Context, actorID ids.ID, hash string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds[actorID] = hash
	return nil
}

// CreateSession stores a session.
func (s *Store) CreateSession(_ context.Context, x identity.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[string(x.TokenHash)] = x
	return nil
}

// SessionByTokenHash returns a non-revoked session and its actor.
func (s *Store) SessionByTokenHash(_ context.Context, hash []byte) (identity.Session, identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	x, ok := s.sessions[string(hash)]
	if !ok || s.revoked[x.ID] {
		return identity.Session{}, identity.Actor{}, db.ErrNotFound
	}
	a, ok := s.actors[x.ActorID]
	if !ok {
		return identity.Session{}, identity.Actor{}, db.ErrNotFound
	}
	return x, a, nil
}

// RevokeSession ends a session.
func (s *Store) RevokeSession(_ context.Context, id ids.ID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[id] = true
	return nil
}

// RevokeSessionsForActor revokes every session of the actor except keep.
func (s *Store) RevokeSessionsForActor(_ context.Context, actorID, keep ids.ID, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.sessions {
		if x.ActorID == actorID && x.ID != keep {
			s.revoked[x.ID] = true
		}
	}
	return nil
}

// ListTokens lists an actor's tokens, oldest first.
func (s *Store) ListTokens(_ context.Context, actorID ids.ID) ([]identity.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []identity.Token
	for _, t := range s.tokens {
		if t.ActorID == actorID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// TokenByID loads a token.
func (s *Store) TokenByID(_ context.Context, id ids.ID) (identity.Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.tokenByID[id]
	if !ok {
		return identity.Token{}, db.ErrNotFound
	}
	return s.tokens[key], nil
}

// RevokeToken revokes a token.
func (s *Store) RevokeToken(_ context.Context, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.tokenByID[id]
	if !ok {
		return nil
	}
	t := s.tokens[key]
	if t.RevokedAt == nil {
		t.RevokedAt = &at
		s.tokens[key] = t
	}
	return nil
}

// SetActorState changes an actor's state.
func (s *Store) SetActorState(_ context.Context, actorID ids.ID, state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[actorID]
	if !ok {
		return db.ErrNotFound
	}
	a.State = state
	s.actors[actorID] = a
	return nil
}

// WorkspaceByID loads a workspace.
func (s *Store) WorkspaceByID(_ context.Context, id ids.ID) (identity.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.workspaces[id]
	if !ok {
		return identity.Workspace{}, db.ErrNotFound
	}
	return w, nil
}

// SetEmergencyStop turns the emergency stop on or off.
func (s *Store) SetEmergencyStop(_ context.Context, workspaceID ids.ID, at *time.Time, by ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[workspaceID]
	if !ok {
		return db.ErrNotFound
	}
	w.EmergencyStopAt = at
	w.EmergencyStopBy = ids.Nil
	if at != nil {
		w.EmergencyStopBy = by
	}
	s.workspaces[workspaceID] = w
	return nil
}
