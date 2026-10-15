// Package postgres implements runs.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/runs"
	"colonyhall/internal/modules/runs/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements runs.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// InsertError stores an error.
func (s *Store) InsertError(ctx context.Context, e runs.Error) error {
	var thread *ids.ID
	if e.ThreadRootID != ids.Nil {
		thread = &e.ThreadRootID
	}
	return s.q(ctx).InsertError(ctx, sqlcgen.InsertErrorParams{ID: e.ID, WorkspaceID: e.WorkspaceID, AgentID: e.AgentID,
		ChannelID: e.ChannelID, ThreadRootID: thread, Summary: e.Summary, Detail: e.Detail, CreatedAt: e.CreatedAt})
}

// ErrorByID loads an error.
func (s *Store) ErrorByID(ctx context.Context, id ids.ID) (runs.Error, error) {
	r, err := s.q(ctx).ErrorByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return runs.Error{}, db.ErrNotFound
	}
	if err != nil {
		return runs.Error{}, err
	}
	e := runs.Error{ID: r.ID, WorkspaceID: r.WorkspaceID, AgentID: r.AgentID, ChannelID: r.ChannelID, Summary: r.Summary,
		Detail: r.Detail, CreatedAt: r.CreatedAt}
	if r.ThreadRootID != nil {
		e.ThreadRootID = *r.ThreadRootID
	}
	return e, nil
}
