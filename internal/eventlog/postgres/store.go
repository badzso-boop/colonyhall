// Package postgres is the Postgres implementation of eventlog.Store.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// appendLockKey is the advisory lock that makes event writes single-writer.
const appendLockKey = 7243100001

// Store implements eventlog.Store on Postgres.
type Store struct{ DB *db.Postgres }

// Append takes the append lock for the rest of the transaction, then
// assigns the next sequence number. Because the lock is held until commit,
// sequence order equals commit order.
func (s *Store) Append(ctx context.Context, e *eventlog.Event) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		q := s.DB.Q(ctx)
		if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, appendLockKey); err != nil {
			return fmt.Errorf("eventlog: lock: %w", err)
		}
		return q.QueryRow(ctx, `
			INSERT INTO event (seq, id, workspace_id, type, actor_id, channel_id,
			                   aggregate_type, aggregate_id, payload, schema_version, at)
			VALUES (nextval('event_seq'), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING seq`,
			e.ID, e.WorkspaceID, e.Type, e.ActorID, nullID(e.ChannelID),
			e.AggregateType, e.AggregateID, e.Payload, e.SchemaVersion, e.At,
		).Scan(&e.Seq)
	})
}

// Since returns events after the given sequence number.
func (s *Store) Since(ctx context.Context, after int64, limit int) ([]eventlog.Event, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `
		SELECT seq, id, workspace_id, type, actor_id, channel_id,
		       aggregate_type, aggregate_id, payload, schema_version, at
		FROM event WHERE seq > $1 ORDER BY seq LIMIT $2`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []eventlog.Event
	for rows.Next() {
		var e eventlog.Event
		var channel *ids.ID
		if err := rows.Scan(&e.Seq, &e.ID, &e.WorkspaceID, &e.Type, &e.ActorID, &channel,
			&e.AggregateType, &e.AggregateID, &e.Payload, &e.SchemaVersion, &e.At); err != nil {
			return nil, err
		}
		if channel != nil {
			e.ChannelID = *channel
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Head returns the newest sequence number.
func (s *Store) Head(ctx context.Context) (int64, error) {
	var seq int64
	err := s.DB.Q(ctx).QueryRow(ctx, `SELECT COALESCE(MAX(seq), 0) FROM event`).Scan(&seq)
	return seq, err
}

// Offset returns a consumer's stored position.
func (s *Store) Offset(ctx context.Context, consumer string) (int64, error) {
	var seq int64
	err := s.DB.Q(ctx).QueryRow(ctx, `SELECT last_seq FROM consumer_offset WHERE consumer_name = $1`, consumer).Scan(&seq)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// SetOffset stores a consumer's position.
func (s *Store) SetOffset(ctx context.Context, consumer string, seq int64) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `
		INSERT INTO consumer_offset (consumer_name, last_seq, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (consumer_name) DO UPDATE SET last_seq = EXCLUDED.last_seq, updated_at = now()`,
		consumer, seq)
	return err
}

func nullID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}
