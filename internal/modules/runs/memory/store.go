// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory reference implementation of runs.Store.
package memory

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements runs.Store in memory.
type Store struct {
	prices   []runs.Price
	currency map[ids.ID]runs.Currency
	mu       sync.RWMutex
	errors   map[ids.ID]runs.Error
	runs     []runs.Run
}

// New returns an empty store.
func New() *Store {
	return &Store{errors: map[ids.ID]runs.Error{}, currency: map[ids.ID]runs.Currency{}}
}

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

// TokensSince returns the sum of all tokens consumed by the agent since the given timestamp.
func (s *Store) TokensSince(_ context.Context, workspaceID, agentID ids.ID, since time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var total int64
	for _, r := range s.runs {
		if r.WorkspaceID == workspaceID && r.AgentID == agentID && !r.CreatedAt.Before(since) {
			total += r.InputTokens + r.OutputTokens + r.CacheReadTokens + r.CacheWriteTokens
		}
	}
	return total, nil
}

// SearchRuns lists runs by filter, newest first.
func (s *Store) SearchRuns(_ context.Context, workspaceID ids.ID, f runs.RunFilter) ([]runs.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []runs.Run
	for _, r := range s.runs {
		switch {
		case r.WorkspaceID != workspaceID,
			f.AgentID != ids.Nil && r.AgentID != f.AgentID,
			f.HostID != ids.Nil && r.HostID != f.HostID,
			f.ChannelID != ids.Nil && r.ChannelID != f.ChannelID,
			f.Status != "" && r.Status != f.Status,
			f.AgentIDs != nil && !slices.Contains(f.AgentIDs, r.AgentID),
			f.From != nil && r.CreatedAt.Before(*f.From),
			f.To != nil && !r.CreatedAt.Before(*f.To):
			continue
		}
		if b := f.Before; b != nil && (r.CreatedAt.After(b.CreatedAt) || (r.CreatedAt.Equal(b.CreatedAt) && r.ID.String() >= b.ID.String())) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID.String() > out[j].ID.String()
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

// UsageRuns lists what usage needs of every run since a time.
func (s *Store) UsageRuns(_ context.Context, workspaceID ids.ID, since time.Time) ([]runs.UsageRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []runs.UsageRun
	for _, r := range s.runs {
		if r.WorkspaceID == workspaceID && !r.CreatedAt.Before(since) {
			out = append(out, runs.UsageRun{AgentID: r.AgentID, HostID: r.HostID, Model: r.Model, Client: r.Client, Status: r.Status,
				DurationMS: r.DurationMS, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, CacheReadTokens: r.CacheReadTokens,
				CacheWriteTokens: r.CacheWriteTokens, CreatedAt: r.CreatedAt})
		}
	}
	return out, nil
}

// InsertPrice adds a row to the price table.
func (s *Store) InsertPrice(_ context.Context, p runs.Price) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices = append(s.prices, p)
	return nil
}

// Prices lists the workspace's price rows, by model and valid_from.
func (s *Store) Prices(_ context.Context, workspaceID ids.ID) ([]runs.Price, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []runs.Price
	for _, p := range s.prices {
		if p.WorkspaceID == workspaceID {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		return out[i].ValidFrom.Before(out[j].ValidFrom)
	})
	return out, nil
}

// Currency returns the workspace's exchange rate.
func (s *Store) Currency(_ context.Context, workspaceID ids.ID) (runs.Currency, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.currency[workspaceID]
	if !ok {
		return runs.Currency{}, db.ErrNotFound
	}
	return c, nil
}

// PutCurrency stores the workspace's exchange rate.
func (s *Store) PutCurrency(_ context.Context, c runs.Currency) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currency[c.WorkspaceID] = c
	return nil
}
