// Package memory is the in-memory reference implementation of identity.Store.
package memory

import (
	"bytes"
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
	revoked    map[ids.ID]time.Time        // session id → when it was revoked
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
		revoked:    map[ids.ID]time.Time{},
	}
}

// CreateWorkspace stores a workspace.
func (s *Store) CreateWorkspace(_ context.Context, w identity.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w = identity.WithDefaults(w)
	for _, other := range s.workspaces {
		if other.Slug == w.Slug {
			return db.ErrConflict
		}
	}
	s.workspaces[w.ID] = w
	return nil
}

// WorkspaceBySlug loads a workspace by its subdomain name.
func (s *Store) WorkspaceBySlug(_ context.Context, slug string) (identity.Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, w := range s.workspaces {
		if w.Slug == slug {
			return w, nil
		}
	}
	return identity.Workspace{}, db.ErrNotFound
}

// SetWorkspaceSlug renames a workspace's subdomain.
func (s *Store) SetWorkspaceSlug(_ context.Context, id ids.ID, slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[id]
	if !ok {
		return db.ErrNotFound
	}
	for _, other := range s.workspaces {
		if other.ID != id && other.Slug == slug {
			return db.ErrConflict
		}
	}
	w.Slug = slug
	s.workspaces[id] = w
	return nil
}

// CreateActor stores an actor, rejecting duplicate handles.
func (s *Store) CreateActor(_ context.Context, a identity.Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, other := range s.actors {
		if other.WorkspaceID == a.WorkspaceID && other.Handle == a.Handle && other.ArchivedAt == nil {
			return db.ErrConflict
		}
	}
	if a.Timezone == "" {
		a.Timezone = identity.DefaultTimezone
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
		if a.WorkspaceID == workspaceID && a.Handle == handle && a.ArchivedAt == nil {
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
// TouchToken records a token's use unless it was recorded after notBefore.
func (s *Store) TouchToken(_ context.Context, hash []byte, at, notBefore time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[string(hash)]
	if ok && (t.LastUsedAt == nil || t.LastUsedAt.Before(notBefore)) {
		t.LastUsedAt = &at
		s.tokens[string(hash)] = t
	}
	return nil
}

// TouchSession records a session's use unless it was recorded after notBefore.
func (s *Store) TouchSession(_ context.Context, id ids.ID, at, notBefore time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, x := range s.sessions {
		if x.ID == id && (x.LastSeenAt == nil || x.LastSeenAt.Before(notBefore)) {
			x.LastSeenAt = &at
			s.sessions[k] = x
		}
	}
	return nil
}

func (s *Store) ActorByTokenHash(_ context.Context, hash []byte, now time.Time) (identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[string(hash)]
	if !ok || t.RevokedAt != nil || (t.ExpiresAt != nil && !t.ExpiresAt.After(now)) {
		return identity.Actor{}, db.ErrNotFound
	}
	a, ok := s.actors[t.ActorID]
	if !ok {
		return identity.Actor{}, db.ErrNotFound
	}
	return a, nil
}

// ListActors lists the workspace's actors that are not archived, sorted by handle.
func (s *Store) ListActors(_ context.Context, workspaceID ids.ID) ([]identity.Actor, error) {
	return s.list(workspaceID, false), nil
}

// ListAllActors lists archived actors too.
func (s *Store) ListAllActors(_ context.Context, workspaceID ids.ID) ([]identity.Actor, error) {
	return s.list(workspaceID, true), nil
}

func (s *Store) list(workspaceID ids.ID, archived bool) []identity.Actor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []identity.Actor
	for _, a := range s.actors {
		if a.WorkspaceID == workspaceID && (archived || a.ArchivedAt == nil) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Handle != out[j].Handle {
			return out[i].Handle < out[j].Handle
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// ArchiveActor stops and archives the actor.
func (s *Store) ArchiveActor(_ context.Context, id, by ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[id]
	if !ok {
		return db.ErrNotFound
	}
	if a.ArchivedAt == nil {
		a.ArchivedAt, a.ArchivedBy, a.State = &at, by, "stopped"
		s.actors[id] = a
	}
	return nil
}

// RevokeActorTokens revokes every API token of the actor.
func (s *Store) RevokeActorTokens(_ context.Context, actorID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, t := range s.tokens {
		if t.ActorID == actorID && t.RevokedAt == nil {
			t.RevokedAt = &at
			s.tokens[k] = t
		}
	}
	return nil
}

// ActorsByHandle finds the handle in every workspace.
func (s *Store) ActorsByHandle(_ context.Context, handle string) ([]identity.Actor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []identity.Actor
	for _, a := range s.actors {
		if a.Handle == handle && a.ArchivedAt == nil {
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
	if _, gone := s.revoked[x.ID]; !ok || gone {
		return identity.Session{}, identity.Actor{}, db.ErrNotFound
	}
	a, ok := s.actors[x.ActorID]
	if !ok {
		return identity.Session{}, identity.Actor{}, db.ErrNotFound
	}
	return x, a, nil
}

// RevokeSession ends a session.
func (s *Store) RevokeSession(_ context.Context, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokeSession(id, at)
	return nil
}

// revokeSession records the first revocation only, like revoked_at IS NULL.
func (s *Store) revokeSession(id ids.ID, at time.Time) {
	if _, done := s.revoked[id]; !done {
		s.revoked[id] = at
	}
}

// RevokeSessionsForActor revokes every session of the actor except keep.
func (s *Store) RevokeSessionsForActor(_ context.Context, actorID, keep ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.sessions {
		if x.ActorID == actorID && x.ID != keep {
			s.revokeSession(x.ID, at)
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
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return bytes.Compare(out[i].ID[:], out[j].ID[:]) < 0 // ids are time-ordered
	})
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

// RevokeHostTokens revokes a host's agent tokens.
func (s *Store) RevokeHostTokens(_ context.Context, hostID, agentID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, t := range s.tokens {
		if t.HostID == hostID && t.RevokedAt == nil && (agentID == ids.Nil || t.ActorID == agentID) {
			t.RevokedAt = &at
			s.tokens[k] = t
		}
	}
	return nil
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

// UpdateActorProfile updates an actor's profile fields.
func (s *Store) UpdateActorProfile(_ context.Context, actorID ids.ID, displayName, desc, avatarEmoji, tz, statusText, color string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[actorID]
	if !ok {
		return db.ErrNotFound
	}
	a.DisplayName = displayName
	a.Description = desc
	a.AvatarEmoji = avatarEmoji
	a.Timezone = tz
	a.StatusText = statusText
	a.Color = color
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

// SessionStats counts each person's sessions alive at now, and their last login.
func (s *Store) SessionStats(_ context.Context, workspaceID ids.ID, now time.Time) (map[ids.ID]identity.SessionStat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[ids.ID]identity.SessionStat{}
	for _, x := range s.sessions {
		a, ok := s.actors[x.ActorID]
		if !ok || a.WorkspaceID != workspaceID {
			continue
		}
		st := out[x.ActorID]
		if _, gone := s.revoked[x.ID]; !gone && x.ExpiresAt.After(now) {
			st.Active++
		}
		if st.LastLogin == nil || x.CreatedAt.After(*st.LastLogin) {
			at := x.CreatedAt
			st.LastLogin = &at
		}
		seen := x.CreatedAt // a login is activity too
		if x.LastSeenAt != nil {
			seen = *x.LastSeenAt
		}
		if st.LastSeen == nil || seen.After(*st.LastSeen) {
			st.LastSeen = &seen
		}
		out[x.ActorID] = st
	}
	return out, nil
}

// SetActorRole changes an actor's role.
func (s *Store) SetActorRole(_ context.Context, id ids.ID, role identity.Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[id]
	if !ok {
		return db.ErrNotFound
	}
	a.Role = role
	s.actors[id] = a
	return nil
}

// SetActorOwner sets or clears an agent's owner.
func (s *Store) SetActorOwner(_ context.Context, id, ownerID ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actors[id]
	if !ok {
		return db.ErrNotFound
	}
	a.OwnerID = ownerID
	s.actors[id] = a
	return nil
}

// DeleteExpiredHostTokens deletes ended guard tokens.
func (s *Store) DeleteExpiredHostTokens(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for k, t := range s.tokens {
		if t.HostID == ids.Nil {
			continue
		}
		if (t.ExpiresAt != nil && t.ExpiresAt.Before(before)) || (t.RevokedAt != nil && t.RevokedAt.Before(before)) {
			delete(s.tokens, k)
			delete(s.tokenByID, t.ID)
			n++
		}
	}
	return n, nil
}

// DeleteEndedSessions deletes sessions that expired or were revoked before the cut-off.
func (s *Store) DeleteEndedSessions(_ context.Context, before time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for k, x := range s.sessions {
		at, revoked := s.revoked[x.ID]
		if x.ExpiresAt.Before(before) || (revoked && at.Before(before)) {
			delete(s.sessions, k)
			delete(s.revoked, x.ID)
			n++
		}
	}
	return n, nil
}
