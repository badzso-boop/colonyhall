// Package postgres implements schedule.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/schedule"
	"colonyhall/internal/modules/schedule/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements schedule.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func nullID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

func toSchedule(r sqlcgen.Schedule) schedule.Schedule {
	sc := schedule.Schedule{ID: r.ID, WorkspaceID: r.WorkspaceID, CreatedBy: r.CreatedBy, ChannelID: r.ChannelID, Body: r.Body,
		Cron: r.Cron, Timezone: r.Timezone, CatchUp: r.CatchUp, State: r.State, NextRunAt: r.NextRunAt, LastRunAt: r.LastRunAt,
		LastError: r.LastError, RunCount: int(r.RunCount), CreatedAt: r.CreatedAt}
	if r.ThreadRootID != nil {
		sc.ThreadRootID = *r.ThreadRootID
	}
	if r.LastMessageID != nil {
		sc.LastMessageID = *r.LastMessageID
	}
	return sc
}

func list(rows []sqlcgen.Schedule, err error) ([]schedule.Schedule, error) {
	if err != nil {
		return nil, err
	}
	out := make([]schedule.Schedule, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSchedule(r))
	}
	return out, nil
}

// Create stores a schedule.
func (s *Store) Create(ctx context.Context, sc schedule.Schedule) error {
	return s.q(ctx).CreateSchedule(ctx, sqlcgen.CreateScheduleParams{ID: sc.ID, WorkspaceID: sc.WorkspaceID, CreatedBy: sc.CreatedBy,
		ChannelID: sc.ChannelID, ThreadRootID: nullID(sc.ThreadRootID), Body: sc.Body, Cron: sc.Cron, Timezone: sc.Timezone,
		CatchUp: sc.CatchUp, State: sc.State, NextRunAt: sc.NextRunAt, CreatedAt: sc.CreatedAt})
}

// ByID loads a schedule.
func (s *Store) ByID(ctx context.Context, id ids.ID) (schedule.Schedule, error) {
	r, err := s.q(ctx).ScheduleByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule.Schedule{}, db.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, err
	}
	return toSchedule(r), nil
}

// List lists a workspace's schedules by next run.
func (s *Store) List(ctx context.Context, workspaceID ids.ID) ([]schedule.Schedule, error) {
	return list(s.q(ctx).ListSchedules(ctx, workspaceID))
}

// Due lists active schedules due at now.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error) {
	return list(s.q(ctx).DueSchedules(ctx, sqlcgen.DueSchedulesParams{Now: now, Lim: int32(limit)}))
}

// Claim moves a due run on, once.
func (s *Store) Claim(ctx context.Context, id ids.ID, dueAt, next time.Time, state string, at time.Time) (bool, error) {
	n, err := s.q(ctx).ClaimSchedule(ctx, sqlcgen.ClaimScheduleParams{Next: next, State: state, At: &at, ID: id, DueAt: dueAt})
	return n == 1, err
}

// Record stores a run's outcome.
func (s *Store) Record(ctx context.Context, id, messageID ids.ID, runErr string) error {
	return s.q(ctx).RecordRun(ctx, sqlcgen.RecordRunParams{MessageID: nullID(messageID), RunError: runErr, ID: id})
}

// SetState changes a schedule's state.
func (s *Store) SetState(ctx context.Context, id ids.ID, state string) error {
	return s.q(ctx).SetScheduleState(ctx, sqlcgen.SetScheduleStateParams{ID: id, State: state})
}
