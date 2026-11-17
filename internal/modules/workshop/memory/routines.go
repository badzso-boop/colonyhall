// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package memory

import (
	"context"
	"sort"
	"time"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// UpsertRoutineState implements workshop.Store.
func (s *Store) UpsertRoutineState(_ context.Context, st workshop.RoutineState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.states {
		if x.ExtensionID == st.ExtensionID && x.Routine == st.Routine {
			s.states[i].NextRunAt = st.NextRunAt
			return nil
		}
	}
	s.states = append(s.states, st)
	return nil
}

// ListRoutineStates implements workshop.Store.
func (s *Store) ListRoutineStates(_ context.Context, workspaceID, extensionID ids.ID) ([]workshop.RoutineState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.RoutineState
	for _, x := range s.states {
		if x.WorkspaceID == workspaceID && x.ExtensionID == extensionID {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Routine < out[j].Routine })
	return out, nil
}

// GetRoutineState implements workshop.Store.
func (s *Store) GetRoutineState(_ context.Context, workspaceID, id ids.ID) (workshop.RoutineState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.states {
		if x.ID == id && x.WorkspaceID == workspaceID {
			return x, nil
		}
	}
	return workshop.RoutineState{}, db.ErrNotFound
}

func (s *Store) updateState(workspaceID, id ids.ID, f func(*workshop.RoutineState)) {
	for i := range s.states {
		if s.states[i].ID == id && s.states[i].WorkspaceID == workspaceID {
			f(&s.states[i])
		}
	}
}

// SetRoutineNext implements workshop.Store.
func (s *Store) SetRoutineNext(_ context.Context, workspaceID, id ids.ID, next, last *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateState(workspaceID, id, func(x *workshop.RoutineState) { x.NextRunAt, x.LastRunAt = next, last })
	return nil
}

// SetRoutineWebhook implements workshop.Store.
func (s *Store) SetRoutineWebhook(_ context.Context, workspaceID, id ids.ID, hash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateState(workspaceID, id, func(x *workshop.RoutineState) { x.WebhookTokenHash = hash })
	return nil
}

// DueRoutines implements workshop.Store.
func (s *Store) DueRoutines(_ context.Context, now time.Time, limit int) ([]workshop.RoutineState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.RoutineState
	for _, x := range s.states {
		if x.NextRunAt != nil && !x.NextRunAt.After(now) {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NextRunAt.Before(*out[j].NextRunAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// InsertRoutineRun implements workshop.Store.
func (s *Store) InsertRoutineRun(_ context.Context, r workshop.RoutineRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.State = workshop.RunPending
	s.runs = append(s.runs, r)
	return nil
}

// ClaimRoutineRuns implements workshop.Store.
func (s *Store) ClaimRoutineRuns(_ context.Context, now time.Time, limit int) ([]workshop.RoutineRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.RoutineRun
	for i := range s.runs {
		if len(out) >= limit {
			break
		}
		if s.runs[i].State == workshop.RunPending {
			t := now
			s.runs[i].State, s.runs[i].StartedAt = workshop.RunRunning, &t
			out = append(out, s.runs[i])
		}
	}
	return out, nil
}

// FinishRoutineRun implements workshop.Store.
func (s *Store) FinishRoutineRun(_ context.Context, workspaceID, id ids.ID, state, errText string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.runs {
		if s.runs[i].ID == id && s.runs[i].WorkspaceID == workspaceID {
			t := at
			s.runs[i].State, s.runs[i].Error, s.runs[i].FinishedAt = state, errText, &t
		}
	}
	return nil
}

// ListRoutineRuns implements workshop.Store.
func (s *Store) ListRoutineRuns(_ context.Context, workspaceID, extensionID ids.ID, limit int) ([]workshop.RoutineRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.RoutineRun
	for i := len(s.runs) - 1; i >= 0 && len(out) < limit; i-- {
		if s.runs[i].WorkspaceID == workspaceID && s.runs[i].ExtensionID == extensionID {
			out = append(out, s.runs[i])
		}
	}
	return out, nil
}
