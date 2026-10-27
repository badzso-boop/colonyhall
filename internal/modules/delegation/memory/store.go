package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/delegation"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type msgTargetKey struct {
	messageID ids.ID
	targetID  ids.ID
}

// Store implements delegation.Store in memory.
type Store struct {
	mu           sync.Mutex
	items        map[ids.ID]delegation.Delegation
	byQuestionID map[ids.ID]ids.ID
	byMsgTarget  map[msgTargetKey]ids.ID
	grants       map[ids.ID]delegation.Grant
	settings     map[ids.ID]delegation.Settings
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{
		items:        make(map[ids.ID]delegation.Delegation),
		byQuestionID: make(map[ids.ID]ids.ID),
		byMsgTarget:  make(map[msgTargetKey]ids.ID),
		grants:       make(map[ids.ID]delegation.Grant),
		settings:     make(map[ids.ID]delegation.Settings),
	}
}

func clone(d delegation.Delegation) delegation.Delegation {
	out := d
	if d.DecidedBy != nil {
		id := *d.DecidedBy
		out.DecidedBy = &id
	}
	if d.DecidedAt != nil {
		t := *d.DecidedAt
		out.DecidedAt = &t
	}
	return out
}

func cloneGrant(g delegation.Grant) delegation.Grant {
	out := g
	if g.RevokedBy != nil {
		id := *g.RevokedBy
		out.RevokedBy = &id
	}
	if g.RevokedAt != nil {
		t := *g.RevokedAt
		out.RevokedAt = &t
	}
	return out
}

func (s *Store) Insert(_ context.Context, d delegation.Delegation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := msgTargetKey{messageID: d.MessageID, targetID: d.ToAgentID}
	if _, exists := s.byMsgTarget[k]; exists {
		return nil // idempotent / conflict ignored
	}

	cloned := clone(d)
	s.items[d.ID] = cloned
	if d.QuestionID != ids.Nil {
		s.byQuestionID[d.QuestionID] = d.ID
	}
	s.byMsgTarget[k] = d.ID
	return nil
}

func (s *Store) GetByID(_ context.Context, id ids.ID) (delegation.Delegation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.items[id]
	if !ok {
		return delegation.Delegation{}, db.ErrNotFound
	}
	return clone(d), nil
}

func (s *Store) GetByQuestionID(_ context.Context, questionID ids.ID) (delegation.Delegation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.byQuestionID[questionID]
	if !ok {
		return delegation.Delegation{}, db.ErrNotFound
	}
	return clone(s.items[id]), nil
}

func (s *Store) GetByMessageAndTarget(_ context.Context, messageID, toAgentID ids.ID) (delegation.Delegation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.byMsgTarget[msgTargetKey{messageID: messageID, targetID: toAgentID}]
	if !ok {
		return delegation.Delegation{}, db.ErrNotFound
	}
	return clone(s.items[id]), nil
}

func (s *Store) UpdateStateConditional(_ context.Context, id ids.ID, expectedState, newState string, decidedBy *ids.ID, decidedAt time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.items[id]
	if !ok {
		return 0, nil
	}
	if d.State != expectedState {
		return 0, nil
	}
	d.State = newState
	if decidedBy != nil {
		idCopy := *decidedBy
		d.DecidedBy = &idCopy
	}
	tCopy := decidedAt
	d.DecidedAt = &tCopy
	s.items[id] = d
	return 1, nil
}

func (s *Store) ApproveViaGrant(_ context.Context, id ids.ID, decidedBy ids.ID, decidedAt time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.items[id]
	if !ok || d.State != delegation.StatePending {
		return 0, nil
	}
	d.State = delegation.StateApproved
	by := decidedBy
	d.DecidedBy = &by
	at := decidedAt
	d.DecidedAt = &at
	d.ViaGrant = true
	s.items[id] = d
	return 1, nil
}

func (s *Store) ListByWorkspace(_ context.Context, workspaceID ids.ID, state *string) ([]delegation.Delegation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []delegation.Delegation
	for _, d := range s.items {
		if d.WorkspaceID != workspaceID {
			continue
		}
		if state != nil && *state != "" && d.State != *state {
			continue
		}
		out = append(out, clone(d))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) ListPendingByPairAndConversation(_ context.Context, workspaceID, agentA, agentB, channelID ids.ID, threadRootID ids.ID) ([]delegation.Delegation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []delegation.Delegation
	for _, d := range s.items {
		if d.WorkspaceID != workspaceID || d.ChannelID != channelID || d.ThreadRootID != threadRootID || d.State != delegation.StatePending {
			continue
		}
		isPair := (d.FromAgentID == agentA && d.ToAgentID == agentB) || (d.FromAgentID == agentB && d.ToAgentID == agentA)
		if !isPair {
			continue
		}
		out = append(out, clone(d))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) InsertGrant(_ context.Context, g delegation.Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.grants[g.ID] = cloneGrant(g)
	return nil
}

func (s *Store) GetGrantByID(_ context.Context, id ids.ID) (delegation.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.grants[id]
	if !ok {
		return delegation.Grant{}, db.ErrNotFound
	}
	return cloneGrant(g), nil
}

func (s *Store) GetActiveGrant(_ context.Context, workspaceID, agentA, agentB, channelID ids.ID, threadRootID ids.ID, now time.Time) (delegation.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	canonicalA, canonicalB := delegation.CanonicalPair(agentA, agentB)
	var matches []delegation.Grant
	for _, g := range s.grants {
		if g.WorkspaceID != workspaceID || g.ChannelID != channelID || g.ThreadRootID != threadRootID {
			continue
		}
		if g.AgentAID != canonicalA || g.AgentBID != canonicalB {
			continue
		}
		if g.RevokedAt != nil || !now.Before(g.ExpiresAt) {
			continue
		}
		matches = append(matches, cloneGrant(g))
	}
	if len(matches) == 0 {
		return delegation.Grant{}, db.ErrNotFound
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ApprovedAt.After(matches[j].ApprovedAt)
	})
	return matches[0], nil
}

func (s *Store) UpdateGrantLastUsed(_ context.Context, id ids.ID, lastUsedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.grants[id]
	if !ok {
		return db.ErrNotFound
	}
	if g.RevokedAt != nil {
		return nil
	}
	g.LastUsedAt = lastUsedAt
	s.grants[id] = g
	return nil
}

func (s *Store) RevokeGrantConditional(_ context.Context, id ids.ID, revokedBy ids.ID, revokedAt time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.grants[id]
	if !ok {
		return 0, nil
	}
	if g.RevokedAt != nil || !revokedAt.Before(g.ExpiresAt) {
		return 0, nil
	}
	rBy := revokedBy
	rAt := revokedAt
	g.RevokedBy = &rBy
	g.RevokedAt = &rAt
	s.grants[id] = g
	return 1, nil
}

func (s *Store) ListGrants(_ context.Context, workspaceID ids.ID, activeOnly bool, now time.Time) ([]delegation.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []delegation.Grant
	for _, g := range s.grants {
		if g.WorkspaceID != workspaceID {
			continue
		}
		if activeOnly {
			if g.RevokedAt != nil || !now.Before(g.ExpiresAt) {
				continue
			}
		}
		out = append(out, cloneGrant(g))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ApprovedAt.After(out[j].ApprovedAt)
	})
	return out, nil
}

func (s *Store) GetSettings(_ context.Context, workspaceID ids.ID) (delegation.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.settings[workspaceID]
	if !ok {
		return delegation.Settings{}, db.ErrNotFound
	}
	return st, nil
}

func (s *Store) UpsertSettings(_ context.Context, st delegation.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings[st.WorkspaceID] = st
	return nil
}
