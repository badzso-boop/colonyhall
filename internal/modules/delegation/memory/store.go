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
}

// New returns an empty in-memory store.
func New() *Store {
	return &Store{
		items:        make(map[ids.ID]delegation.Delegation),
		byQuestionID: make(map[ids.ID]ids.ID),
		byMsgTarget:  make(map[msgTargetKey]ids.ID),
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
