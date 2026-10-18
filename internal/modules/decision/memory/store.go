package memory

import (
	"context"
	"strings"
	"sync"

	"colonyhall/internal/modules/decision"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements an in-memory decision.Store.
type Store struct {
	mu        sync.RWMutex
	decisions []decision.Decision
}

var _ decision.Store = (*Store)(nil)

// New creates an empty in-memory decision store.
func New() *Store {
	return &Store{}
}

func (s *Store) Insert(_ context.Context, d decision.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.decisions {
		if existing.WorkspaceID == d.WorkspaceID && existing.Number == d.Number {
			return db.ErrConflict
		}
	}
	s.decisions = append(s.decisions, d)
	return nil
}

func (s *Store) NextNumber(_ context.Context, workspaceID ids.ID) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	maxNum := 0
	for _, d := range s.decisions {
		if d.WorkspaceID == workspaceID && d.Number > maxNum {
			maxNum = d.Number
		}
	}
	return maxNum + 1, nil
}

func (s *Store) GetByNumber(_ context.Context, workspaceID ids.ID, number int) (decision.Decision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.decisions {
		if d.WorkspaceID == workspaceID && d.Number == number {
			res := d
			for _, other := range s.decisions {
				if other.WorkspaceID == workspaceID && other.Supersedes != nil && *other.Supersedes == d.Number {
					n := other.Number
					res.SupersededBy = &n
					break
				}
			}
			return res, nil
		}
	}
	return decision.Decision{}, db.ErrNotFound
}

func (s *Store) List(_ context.Context, workspaceID ids.ID, limit int, query string, validOnly bool) ([]decision.Decision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	supersededMap := make(map[int]int)
	for _, other := range s.decisions {
		if other.WorkspaceID == workspaceID && other.Supersedes != nil {
			supersededMap[*other.Supersedes] = other.Number
		}
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var matched []decision.Decision
	for i := len(s.decisions) - 1; i >= 0; i-- {
		d := s.decisions[i]
		if d.WorkspaceID != workspaceID {
			continue
		}
		supBy, isSuperseded := supersededMap[d.Number]
		if isSuperseded {
			n := supBy
			d.SupersededBy = &n
		}
		if validOnly && isSuperseded {
			continue
		}
		if q != "" {
			tMatch := strings.Contains(strings.ToLower(d.Text), q)
			rMatch := strings.Contains(strings.ToLower(d.Rationale), q)
			if !tMatch && !rMatch {
				continue
			}
		}
		matched = append(matched, d)
		if limit > 0 && len(matched) >= limit {
			break
		}
	}
	return matched, nil
}
