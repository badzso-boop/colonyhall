// Package memory is the in-memory reference implementation of runs.Store.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements runs.Store in memory.
type Store struct {
	mu     sync.RWMutex
	errors map[ids.ID]runs.Error
	runs   []runs.Run
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

// InsertRun stores an agent run.
func (s *Store) InsertRun(_ context.Context, r runs.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, r)
	return nil
}

// ListRuns loads runs for a workspace, optionally filtered by agent_id, newest first.
func (s *Store) ListRuns(_ context.Context, workspaceID ids.ID, agentID ids.ID, limit int) ([]runs.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matches []runs.Run
	for _, r := range s.runs {
		if r.WorkspaceID != workspaceID {
			continue
		}
		if agentID != ids.Nil && r.AgentID != agentID {
			continue
		}
		matches = append(matches, r)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].CreatedAt.Equal(matches[j].CreatedAt) {
			return matches[i].ID.String() > matches[j].ID.String()
		}
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})

	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	out := make([]runs.Run, len(matches))
	copy(out, matches)
	return out, nil
}

// Usage calculates daily usage per agent in Europe/Budapest since the given timestamp.
func (s *Store) Usage(_ context.Context, workspaceID ids.ID, since time.Time) ([]runs.DayUsage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	type groupKey struct {
		agentID ids.ID
		day     string
	}
	type groupVal struct {
		inputTokens      int64
		outputTokens     int64
		cacheReadTokens  int64
		cacheWriteTokens int64
		runCount         int
		errorCount       int
	}

	groups := make(map[groupKey]*groupVal)
	for _, r := range s.runs {
		if r.WorkspaceID != workspaceID {
			continue
		}
		if r.CreatedAt.Before(since) {
			continue
		}
		day := r.CreatedAt.In(runs.Budapest).Format("2006-01-02")
		k := groupKey{agentID: r.AgentID, day: day}
		gv := groups[k]
		if gv == nil {
			gv = &groupVal{}
			groups[k] = gv
		}
		gv.inputTokens += r.InputTokens
		gv.outputTokens += r.OutputTokens
		gv.cacheReadTokens += r.CacheReadTokens
		gv.cacheWriteTokens += r.CacheWriteTokens
		gv.runCount++
		if r.Status == runs.StatusError {
			gv.errorCount++
		}
	}

	out := make([]runs.DayUsage, 0, len(groups))
	for k, gv := range groups {
		total := gv.inputTokens + gv.outputTokens + gv.cacheReadTokens + gv.cacheWriteTokens
		out = append(out, runs.DayUsage{
			AgentID:          k.agentID,
			Day:              k.day,
			InputTokens:      gv.inputTokens,
			OutputTokens:     gv.outputTokens,
			CacheReadTokens:  gv.cacheReadTokens,
			CacheWriteTokens: gv.cacheWriteTokens,
			TotalTokens:      total,
			RunCount:         gv.runCount,
			ErrorCount:       gv.errorCount,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Day == out[j].Day {
			return out[i].AgentID.String() < out[j].AgentID.String()
		}
		return out[i].Day > out[j].Day
	})

	return out, nil
}
