// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package postgres

import (
	"context"
	"time"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/modules/workshop/postgres/sqlcgen"
	"colonyhall/internal/platform/ids"
)

func routineState(r sqlcgen.RoutineState) workshop.RoutineState {
	return workshop.RoutineState{ID: r.ID, WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID, Routine: r.Routine,
		NextRunAt: r.NextRunAt, LastRunAt: r.LastRunAt, WebhookTokenHash: r.WebhookTokenHash}
}

func routineRun(r sqlcgen.RoutineRun) workshop.RoutineRun {
	return workshop.RoutineRun{ID: r.ID, WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID, Routine: r.Routine, Trigger: r.Trigger,
		Input: r.Input, State: r.State, Error: r.Error, CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt}
}

// UpsertRoutineState implements workshop.Store.
func (s *Store) UpsertRoutineState(ctx context.Context, st workshop.RoutineState) error {
	return s.q(ctx).UpsertRoutineState(ctx, sqlcgen.UpsertRoutineStateParams{ID: st.ID, WorkspaceID: st.WorkspaceID,
		ExtensionID: st.ExtensionID, Routine: st.Routine, NextRunAt: st.NextRunAt})
}

// ListRoutineStates implements workshop.Store.
func (s *Store) ListRoutineStates(ctx context.Context, workspaceID, extensionID ids.ID) ([]workshop.RoutineState, error) {
	rs, err := s.q(ctx).ListRoutineStates(ctx, sqlcgen.ListRoutineStatesParams{WorkspaceID: workspaceID, ExtensionID: extensionID})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.RoutineState, len(rs))
	for i, r := range rs {
		out[i] = routineState(r)
	}
	return out, nil
}

// GetRoutineState implements workshop.Store.
func (s *Store) GetRoutineState(ctx context.Context, workspaceID, id ids.ID) (workshop.RoutineState, error) {
	r, err := s.q(ctx).GetRoutineState(ctx, sqlcgen.GetRoutineStateParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return workshop.RoutineState{}, notFound(err)
	}
	return routineState(r), nil
}

// SetRoutineNext implements workshop.Store.
func (s *Store) SetRoutineNext(ctx context.Context, workspaceID, id ids.ID, next, last *time.Time) error {
	return s.q(ctx).SetRoutineNext(ctx, sqlcgen.SetRoutineNextParams{WorkspaceID: workspaceID, ID: id, NextRunAt: next, LastRunAt: last})
}

// SetRoutineWebhook implements workshop.Store.
func (s *Store) SetRoutineWebhook(ctx context.Context, workspaceID, id ids.ID, hash []byte) error {
	return s.q(ctx).SetRoutineWebhook(ctx, sqlcgen.SetRoutineWebhookParams{WorkspaceID: workspaceID, ID: id, WebhookTokenHash: hash})
}

// DueRoutines implements workshop.Store.
func (s *Store) DueRoutines(ctx context.Context, now time.Time, limit int) ([]workshop.RoutineState, error) {
	rs, err := s.q(ctx).DueRoutines(ctx, sqlcgen.DueRoutinesParams{NextRunAt: &now, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.RoutineState, len(rs))
	for i, r := range rs {
		out[i] = routineState(r)
	}
	return out, nil
}

// InsertRoutineRun implements workshop.Store.
func (s *Store) InsertRoutineRun(ctx context.Context, r workshop.RoutineRun) error {
	return s.q(ctx).InsertRoutineRun(ctx, sqlcgen.InsertRoutineRunParams{ID: r.ID, WorkspaceID: r.WorkspaceID, ExtensionID: r.ExtensionID,
		Routine: r.Routine, Trigger: r.Trigger, Input: r.Input, CreatedAt: r.CreatedAt})
}

// ClaimRoutineRuns implements workshop.Store.
func (s *Store) ClaimRoutineRuns(ctx context.Context, now time.Time, limit int) ([]workshop.RoutineRun, error) {
	rs, err := s.q(ctx).ClaimRoutineRuns(ctx, sqlcgen.ClaimRoutineRunsParams{StartedAt: &now, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.RoutineRun, len(rs))
	for i, r := range rs {
		out[i] = routineRun(r)
	}
	return out, nil
}

// FinishRoutineRun implements workshop.Store.
func (s *Store) FinishRoutineRun(ctx context.Context, workspaceID, id ids.ID, state, errText string, at time.Time) error {
	return s.q(ctx).FinishRoutineRun(ctx, sqlcgen.FinishRoutineRunParams{WorkspaceID: workspaceID, ID: id, State: state, Error: errText, FinishedAt: &at})
}

// ListRoutineRuns implements workshop.Store.
func (s *Store) ListRoutineRuns(ctx context.Context, workspaceID, extensionID ids.ID, limit int) ([]workshop.RoutineRun, error) {
	rs, err := s.q(ctx).ListRoutineRuns(ctx, sqlcgen.ListRoutineRunsParams{WorkspaceID: workspaceID, ExtensionID: extensionID, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.RoutineRun, len(rs))
	for i, r := range rs {
		out[i] = routineRun(r)
	}
	return out, nil
}
