// Package postgres implements audit.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/audit"
	"colonyhall/internal/modules/audit/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements audit.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Insert stores an entry once per seq.
func (s *Store) Insert(ctx context.Context, e audit.Entry) error {
	return s.q(ctx).InsertEntry(ctx, sqlcgen.InsertEntryParams{
		Seq: e.Seq, WorkspaceID: e.WorkspaceID, ActorID: e.ActorID, Action: e.Action, TargetType: e.TargetType,
		TargetID: e.TargetID, Payload: e.Payload, At: e.At, PrevHash: e.PrevHash, Hash: e.Hash,
	})
}

// Last returns the newest entry of the workspace.
func (s *Store) Last(ctx context.Context, workspaceID ids.ID) (audit.Entry, error) {
	r, err := s.q(ctx).LastEntry(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return audit.Entry{}, db.ErrNotFound
	}
	return toEntry(r), err
}

// Page returns entries before a seq, newest first.
func (s *Store) Page(ctx context.Context, workspaceID ids.ID, before int64, limit int) ([]audit.Entry, error) {
	return many(s.q(ctx).PageEntries(ctx, sqlcgen.PageEntriesParams{WorkspaceID: workspaceID, Before: before, PageSize: int32(limit)}))
}

// Forward returns entries after a seq, oldest first.
func (s *Store) Forward(ctx context.Context, workspaceID ids.ID, after int64, limit int) ([]audit.Entry, error) {
	return many(s.q(ctx).ForwardEntries(ctx, sqlcgen.ForwardEntriesParams{WorkspaceID: workspaceID, Seq: after, Limit: int32(limit)}))
}

func toEntry(r sqlcgen.AuditLog) audit.Entry {
	return audit.Entry{Seq: r.Seq, WorkspaceID: r.WorkspaceID, ActorID: r.ActorID, Action: r.Action,
		TargetType: r.TargetType, TargetID: r.TargetID, Payload: r.Payload, At: r.At, PrevHash: r.PrevHash, Hash: r.Hash}
}

func many(rows []sqlcgen.AuditLog, err error) ([]audit.Entry, error) {
	if err != nil {
		return nil, err
	}
	out := make([]audit.Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, toEntry(r))
	}
	return out, nil
}
