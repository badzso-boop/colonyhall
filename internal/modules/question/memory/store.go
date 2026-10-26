package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store is an in-memory thread-safe implementation of question.Store.
type Store struct {
	mu        sync.Mutex
	questions map[ids.ID]question.Question
	byNum     map[string]ids.ID // key: workspace_id + ":" + number
}

// New returns a fresh in-memory question store.
func New() *Store {
	return &Store{
		questions: make(map[ids.ID]question.Question),
		byNum:     make(map[string]ids.ID),
	}
}

var _ question.Store = (*Store)(nil)

func numKey(wID ids.ID, n int) string {
	return fmt.Sprintf("%s:%d", wID, n)
}

func cloneQuestion(q question.Question) question.Question {
	c := q
	if len(q.Options) > 0 {
		c.Options = make([]string, len(q.Options))
		copy(c.Options, q.Options)
	}
	if q.ToActorID != nil {
		id := *q.ToActorID
		c.ToActorID = &id
	}
	if q.DefaultOption != nil {
		s := *q.DefaultOption
		c.DefaultOption = &s
	}
	if q.Deadline != nil {
		t := *q.Deadline
		c.Deadline = &t
	}
	if q.Answer != nil {
		s := *q.Answer
		c.Answer = &s
	}
	if q.AnsweredBy != nil {
		id := *q.AnsweredBy
		c.AnsweredBy = &id
	}
	if q.AnsweredAt != nil {
		t := *q.AnsweredAt
		c.AnsweredAt = &t
	}
	if q.EscalatedAt != nil {
		t := *q.EscalatedAt
		c.EscalatedAt = &t
	}
	if q.WithdrawnAt != nil {
		t := *q.WithdrawnAt
		c.WithdrawnAt = &t
	}
	if q.ExpiredAt != nil {
		t := *q.ExpiredAt
		c.ExpiredAt = &t
	}
	return c
}

func (s *Store) Insert(_ context.Context, q question.Question) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := numKey(q.WorkspaceID, q.Number)
	if _, exists := s.byNum[k]; exists {
		return db.ErrConflict
	}
	if _, exists := s.questions[q.ID]; exists {
		return db.ErrConflict
	}
	cl := cloneQuestion(q)
	s.questions[q.ID] = cl
	s.byNum[k] = q.ID
	return nil
}

func (s *Store) NextNumber(_ context.Context, workspaceID ids.ID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxNum := 0
	for _, q := range s.questions {
		if q.WorkspaceID == workspaceID && q.Number > maxNum {
			maxNum = q.Number
		}
	}
	return maxNum + 1, nil
}

func (s *Store) CountOpenByThreadAsker(_ context.Context, threadRootID, askedBy ids.ID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for _, q := range s.questions {
		if q.ThreadRootID == threadRootID && q.AskedBy == askedBy {
			if q.State == question.StateOpen || q.State == question.StateEscalated {
				count++
			}
		}
	}
	return count, nil
}

func (s *Store) GetByID(_ context.Context, id ids.ID) (question.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.questions[id]
	if !ok {
		return question.Question{}, db.ErrNotFound
	}
	return cloneQuestion(q), nil
}

func (s *Store) GetByNumber(_ context.Context, workspaceID ids.ID, number int) (question.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.byNum[numKey(workspaceID, number)]
	if !ok {
		return question.Question{}, db.ErrNotFound
	}
	return cloneQuestion(s.questions[id]), nil
}

func (s *Store) List(_ context.Context, workspaceID ids.ID, state *string) ([]question.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []question.Question
	for _, q := range s.questions {
		if q.WorkspaceID != workspaceID {
			continue
		}
		if state != nil && q.State != *state {
			continue
		}
		out = append(out, cloneQuestion(q))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) Answer(_ context.Context, id ids.ID, answer string, answeredBy *ids.ID, answeredAt time.Time) (question.Question, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.questions[id]
	if !ok {
		return question.Question{}, false, db.ErrNotFound
	}
	if q.State != question.StateOpen && q.State != question.StateEscalated {
		return question.Question{}, false, nil
	}
	q.State = question.StateAnswered
	ans := answer
	q.Answer = &ans
	if answeredBy != nil {
		ab := *answeredBy
		q.AnsweredBy = &ab
	} else {
		q.AnsweredBy = nil
	}
	aat := answeredAt
	q.AnsweredAt = &aat
	s.questions[id] = q
	return cloneQuestion(q), true, nil
}

func (s *Store) Withdraw(_ context.Context, id ids.ID, withdrawnAt time.Time) (question.Question, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.questions[id]
	if !ok {
		return question.Question{}, false, db.ErrNotFound
	}
	if q.State != question.StateOpen && q.State != question.StateEscalated {
		return question.Question{}, false, nil
	}
	q.State = question.StateWithdrawn
	wat := withdrawnAt
	q.WithdrawnAt = &wat
	s.questions[id] = q
	return cloneQuestion(q), true, nil
}

func (s *Store) Escalate(_ context.Context, id ids.ID, escalatedAt time.Time) (question.Question, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.questions[id]
	if !ok {
		return question.Question{}, false, db.ErrNotFound
	}
	if q.State != question.StateOpen {
		return question.Question{}, false, nil
	}
	q.State = question.StateEscalated
	eat := escalatedAt
	q.EscalatedAt = &eat
	s.questions[id] = q
	return cloneQuestion(q), true, nil
}

func (s *Store) Expire(_ context.Context, id ids.ID, expiredAt time.Time) (question.Question, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.questions[id]
	if !ok {
		return question.Question{}, false, db.ErrNotFound
	}
	if q.State != question.StateOpen && q.State != question.StateEscalated {
		return question.Question{}, false, nil
	}
	q.State = question.StateExpired
	xat := expiredAt
	q.ExpiredAt = &xat
	s.questions[id] = q
	return cloneQuestion(q), true, nil
}

func (s *Store) Reopen(_ context.Context, id ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	q, ok := s.questions[id]
	if !ok {
		return db.ErrNotFound
	}
	q.State = question.StateOpen
	q.Answer = nil
	q.AnsweredBy = nil
	q.AnsweredAt = nil
	s.questions[id] = q
	return nil
}

func (s *Store) Due(_ context.Context, escalateBefore, deadlineBefore time.Time, limit int) ([]question.Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []question.Question
	for _, q := range s.questions {
		if q.State != question.StateOpen {
			continue
		}
		if q.ToHuman && !q.AllowExpire {
			continue
		}
		isEscalate := !q.ToHuman && !q.CreatedAt.After(escalateBefore)
		isDeadline := q.Deadline != nil && !q.Deadline.After(deadlineBefore)
		if isEscalate || isDeadline {
			out = append(out, cloneQuestion(q))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
