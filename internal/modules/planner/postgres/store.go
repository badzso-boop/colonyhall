// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the PostgreSQL planner.Store.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/planner"
	"colonyhall/internal/modules/planner/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements planner.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ErrNotFound
	}
	return err
}

func task(r sqlcgen.PlannerTask) planner.Task {
	return planner.Task{ID: r.ID, WorkspaceID: r.WorkspaceID, OwnerID: r.OwnerID, Title: r.Title, Description: r.Description,
		Status: r.Status, Position: r.Position, StartedAt: r.StartedAt, DoneAt: r.DoneAt, StatusChangedAt: r.StatusChangedAt,
		DeletedAt: r.DeletedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

// InsertTask implements planner.Store.
func (s *Store) InsertTask(ctx context.Context, t planner.Task) error {
	err := s.q(ctx).InsertTask(ctx, sqlcgen.InsertTaskParams{ID: t.ID, WorkspaceID: t.WorkspaceID, OwnerID: t.OwnerID,
		Title: t.Title, Description: t.Description, Status: t.Status, Position: t.Position, StartedAt: t.StartedAt,
		DoneAt: t.DoneAt, StatusChangedAt: t.StatusChangedAt, DeletedAt: t.DeletedAt, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// UpdateTask implements planner.Store.
func (s *Store) UpdateTask(ctx context.Context, t planner.Task) error {
	n, err := s.q(ctx).UpdateTask(ctx, sqlcgen.UpdateTaskParams{ID: t.ID, Title: t.Title, Description: t.Description,
		Status: t.Status, Position: t.Position, StartedAt: t.StartedAt, DoneAt: t.DoneAt,
		StatusChangedAt: t.StatusChangedAt, DeletedAt: t.DeletedAt, UpdatedAt: t.UpdatedAt})
	if err == nil && n == 0 {
		return db.ErrNotFound
	}
	return err
}

// TaskByID implements planner.Store.
func (s *Store) TaskByID(ctx context.Context, id ids.ID) (planner.Task, error) {
	r, err := s.q(ctx).TaskByID(ctx, id)
	if err != nil {
		return planner.Task{}, notFound(err)
	}
	return task(r), nil
}

// Tasks implements planner.Store.
func (s *Store) Tasks(ctx context.Context, ownerID ids.ID, doneSince time.Time) ([]planner.Task, error) {
	rows, err := s.q(ctx).Tasks(ctx, sqlcgen.TasksParams{OwnerID: ownerID, DoneSince: &doneSince})
	if err != nil {
		return nil, err
	}
	out := make([]planner.Task, 0, len(rows))
	for _, r := range rows {
		out = append(out, task(r))
	}
	return out, nil
}

// MinPosition implements planner.Store.
func (s *Store) MinPosition(ctx context.Context, ownerID ids.ID) (float64, bool, error) {
	r, err := s.q(ctx).MinPosition(ctx, ownerID)
	if err != nil {
		return 0, false, err
	}
	return r.MinPos, r.N > 0, nil
}

func access(r sqlcgen.PlannerAccess) planner.Access {
	return planner.Access{OwnerID: r.OwnerID, WorkspaceID: r.WorkspaceID, TaskAll: r.TaskAll,
		TaskAgentIDs: r.TaskAgentIds, CalAgentIDs: r.CalAgentIds, UpdatedAt: r.UpdatedAt}
}

// Access implements planner.Store.
func (s *Store) Access(ctx context.Context, ownerID ids.ID) (planner.Access, error) {
	r, err := s.q(ctx).Access(ctx, ownerID)
	if err != nil {
		return planner.Access{}, notFound(err)
	}
	return access(r), nil
}

// PutAccess implements planner.Store.
func (s *Store) PutAccess(ctx context.Context, a planner.Access) error {
	ta, ca := a.TaskAgentIDs, a.CalAgentIDs
	if ta == nil {
		ta = []ids.ID{}
	}
	if ca == nil {
		ca = []ids.ID{}
	}
	return s.q(ctx).PutAccess(ctx, sqlcgen.PutAccessParams{OwnerID: a.OwnerID, WorkspaceID: a.WorkspaceID,
		TaskAll: a.TaskAll, TaskAgentIds: ta, CalAgentIds: ca, UpdatedAt: a.UpdatedAt})
}

// AccessList implements planner.Store.
func (s *Store) AccessList(ctx context.Context, workspaceID ids.ID) ([]planner.Access, error) {
	rows, err := s.q(ctx).AccessList(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]planner.Access, 0, len(rows))
	for _, r := range rows {
		out = append(out, access(r))
	}
	return out, nil
}

func calendar(r sqlcgen.PlannerCalendar) planner.Calendar {
	return planner.Calendar{ID: r.ID, WorkspaceID: r.WorkspaceID, OwnerID: r.OwnerID, Name: r.Name, URL: r.Url,
		Colour: r.Colour, LastFetchedAt: r.LastFetchedAt, LastError: r.LastError, CreatedAt: r.CreatedAt}
}

func calendars(rows []sqlcgen.PlannerCalendar) []planner.Calendar {
	out := make([]planner.Calendar, 0, len(rows))
	for _, r := range rows {
		out = append(out, calendar(r))
	}
	return out
}

// InsertCalendar implements planner.Store.
func (s *Store) InsertCalendar(ctx context.Context, c planner.Calendar) error {
	return s.q(ctx).InsertCalendar(ctx, sqlcgen.InsertCalendarParams{ID: c.ID, WorkspaceID: c.WorkspaceID, OwnerID: c.OwnerID,
		Name: c.Name, Url: c.URL, Colour: c.Colour, LastFetchedAt: c.LastFetchedAt, LastError: c.LastError, CreatedAt: c.CreatedAt})
}

// CalendarByID implements planner.Store.
func (s *Store) CalendarByID(ctx context.Context, id ids.ID) (planner.Calendar, error) {
	r, err := s.q(ctx).CalendarByID(ctx, id)
	if err != nil {
		return planner.Calendar{}, notFound(err)
	}
	return calendar(r), nil
}

// Calendars implements planner.Store.
func (s *Store) Calendars(ctx context.Context, ownerID ids.ID) ([]planner.Calendar, error) {
	rows, err := s.q(ctx).Calendars(ctx, ownerID)
	return calendars(rows), err
}

// CalendarsDue implements planner.Store.
func (s *Store) CalendarsDue(ctx context.Context, before time.Time) ([]planner.Calendar, error) {
	rows, err := s.q(ctx).CalendarsDue(ctx, &before)
	return calendars(rows), err
}

// DeleteCalendar implements planner.Store (events go with it: ON DELETE CASCADE).
func (s *Store) DeleteCalendar(ctx context.Context, id ids.ID) error {
	return s.q(ctx).DeleteCalendar(ctx, id)
}

// SetCalendarFetch implements planner.Store.
func (s *Store) SetCalendarFetch(ctx context.Context, id ids.ID, at time.Time, fetchErr string) error {
	n, err := s.q(ctx).SetCalendarFetch(ctx, sqlcgen.SetCalendarFetchParams{ID: id, LastFetchedAt: &at, LastError: fetchErr})
	if err == nil && n == 0 {
		return db.ErrNotFound
	}
	return err
}

// ReplaceEvents implements planner.Store; callers run it in a transaction.
func (s *Store) ReplaceEvents(ctx context.Context, calendarID ids.ID, events []planner.Event) error {
	q := s.q(ctx)
	if err := q.DeleteEvents(ctx, calendarID); err != nil {
		return err
	}
	for _, e := range events {
		if err := q.InsertEvent(ctx, sqlcgen.InsertEventParams{CalendarID: calendarID, OwnerID: e.OwnerID, Title: e.Title,
			StartsAt: e.Start, EndsAt: e.End, AllDay: e.AllDay, Location: e.Location, Description: e.Description}); err != nil {
			return err
		}
	}
	return nil
}

// Events implements planner.Store.
func (s *Store) Events(ctx context.Context, ownerID ids.ID, from, to time.Time) ([]planner.Event, error) {
	rows, err := s.q(ctx).Events(ctx, sqlcgen.EventsParams{OwnerID: ownerID, FromAt: from, ToAt: to})
	if err != nil {
		return nil, err
	}
	out := make([]planner.Event, 0, len(rows))
	for _, r := range rows {
		out = append(out, planner.Event{CalendarID: r.CalendarID, OwnerID: r.OwnerID, Title: r.Title, Start: r.StartsAt,
			End: r.EndsAt, AllDay: r.AllDay, Location: r.Location, Description: r.Description})
	}
	return out, nil
}
