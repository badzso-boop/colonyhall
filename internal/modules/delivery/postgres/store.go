// Package postgres implements delivery.Store on Postgres.
package postgres

import (
	"context"
	"time"

	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements delivery.Store.
type Store struct{ DB *db.Postgres }

// Insert stores a delivery once per (recipient, item).
func (s *Store) Insert(ctx context.Context, d delivery.Delivery) (bool, error) {
	tag, err := s.DB.Q(ctx).Exec(ctx, `
		INSERT INTO delivery (id, workspace_id, recipient_id, item_kind, item_id, channel_id, flags, event_seq, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (recipient_id, item_kind, item_id) DO NOTHING`,
		d.ID, d.WorkspaceID, d.RecipientID, d.ItemKind, d.ItemID, d.ChannelID, int16(d.Flags), d.EventSeq, d.CreatedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Pending returns unacknowledged deliveries, oldest first.
func (s *Store) Pending(ctx context.Context, recipientID ids.ID, limit int) ([]delivery.Delivery, int, error) {
	q := s.DB.Q(ctx)
	var total int
	if err := q.QueryRow(ctx,
		`SELECT count(*) FROM delivery WHERE recipient_id = $1 AND acked_at IS NULL`, recipientID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `
		SELECT id, workspace_id, recipient_id, item_kind, item_id, channel_id, flags, event_seq, created_at, acked_at
		FROM delivery WHERE recipient_id = $1 AND acked_at IS NULL
		ORDER BY id LIMIT $2`, recipientID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []delivery.Delivery
	for rows.Next() {
		var d delivery.Delivery
		var flags int16
		var channel *ids.ID
		if err := rows.Scan(&d.ID, &d.WorkspaceID, &d.RecipientID, &d.ItemKind, &d.ItemID, &channel,
			&flags, &d.EventSeq, &d.CreatedAt, &d.AckedAt); err != nil {
			return nil, 0, err
		}
		d.Flags = delivery.Flags(flags)
		if channel != nil {
			d.ChannelID = *channel
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// Ack marks deliveries as handled.
func (s *Store) Ack(ctx context.Context, recipientID ids.ID, idList []ids.ID, at time.Time) (int, error) {
	tag, err := s.DB.Q(ctx).Exec(ctx, `
		UPDATE delivery SET acked_at = $3
		WHERE recipient_id = $1 AND id = ANY($2) AND acked_at IS NULL`, recipientID, idList, at)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
