// Package postgres implements eventlog.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/eventlog/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements eventlog.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Append takes the append lock for the rest of the transaction, then
// assigns the next sequence number. Because the lock is held until commit,
// sequence order equals commit order.
func (s *Store) Append(ctx context.Context, e *eventlog.Event) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		q := s.q(ctx)
		if err := q.LockAppend(ctx); err != nil {
			return fmt.Errorf("eventlog: lock: %w", err)
		}
		seq, err := q.InsertEvent(ctx, sqlcgen.InsertEventParams{
			ID: e.ID, WorkspaceID: e.WorkspaceID, Type: e.Type, ActorID: e.ActorID,
			ChannelID: nullID(e.ChannelID), AggregateType: e.AggregateType, AggregateID: e.AggregateID,
			Payload: e.Payload, SchemaVersion: int32(e.SchemaVersion), At: e.At,
		})
		e.Seq = seq
		return err
	})
}

// Since returns events after the given sequence number.
func (s *Store) Since(ctx context.Context, after int64, limit int) ([]eventlog.Event, error) {
	rows, err := s.q(ctx).EventsSince(ctx, sqlcgen.EventsSinceParams{Seq: after, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]eventlog.Event, 0, len(rows))
	for _, r := range rows {
		e := eventlog.Event{Seq: r.Seq, ID: r.ID, WorkspaceID: r.WorkspaceID, Type: r.Type, ActorID: r.ActorID,
			AggregateType: r.AggregateType, AggregateID: r.AggregateID, Payload: r.Payload,
			SchemaVersion: int(r.SchemaVersion), At: r.At}
		if r.ChannelID != nil {
			e.ChannelID = *r.ChannelID
		}
		out = append(out, e)
	}
	return out, nil
}

// Head returns the newest sequence number.
func (s *Store) Head(ctx context.Context) (int64, error) { return s.q(ctx).Head(ctx) }

// Offset returns a consumer's stored position.
func (s *Store) Offset(ctx context.Context, consumer string) (int64, error) {
	seq, err := s.q(ctx).ConsumerOffset(ctx, consumer)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// SetOffset stores a consumer's position.
func (s *Store) SetOffset(ctx context.Context, consumer string, seq int64) error {
	return s.q(ctx).SetConsumerOffset(ctx, sqlcgen.SetConsumerOffsetParams{ConsumerName: consumer, LastSeq: seq})
}

func nullID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}
