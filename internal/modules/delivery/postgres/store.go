// Package postgres implements delivery.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"time"

	"colonyhall/internal/modules/delivery"
	"colonyhall/internal/modules/delivery/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements delivery.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Insert stores a delivery once per (recipient, item).
func (s *Store) Insert(ctx context.Context, d delivery.Delivery) (bool, error) {
	var channel *ids.ID
	if d.ChannelID != ids.Nil {
		channel = &d.ChannelID
	}
	n, err := s.q(ctx).InsertDelivery(ctx, sqlcgen.InsertDeliveryParams{
		ID: d.ID, WorkspaceID: d.WorkspaceID, RecipientID: d.RecipientID, ItemKind: d.ItemKind, ItemID: d.ItemID,
		ChannelID: channel, Flags: int16(d.Flags), EventSeq: d.EventSeq, CreatedAt: d.CreatedAt,
		FromActorID: nullID(d.FromActorID), DelegatedBy: nullID(d.DelegatedBy),
	})
	return n == 1, err
}

// Pending returns unacknowledged deliveries, oldest first.
func (s *Store) Pending(ctx context.Context, recipientID ids.ID, limit int) ([]delivery.Delivery, int, error) {
	q := s.q(ctx)
	total, err := q.CountPending(ctx, recipientID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := q.Pending(ctx, sqlcgen.PendingParams{RecipientID: recipientID, Limit: int32(limit)})
	if err != nil {
		return nil, 0, err
	}
	out := make([]delivery.Delivery, 0, len(rows))
	for _, r := range rows {
		d := delivery.Delivery{ID: r.ID, WorkspaceID: r.WorkspaceID, RecipientID: r.RecipientID, ItemKind: r.ItemKind,
			ItemID: r.ItemID, Flags: delivery.Flags(r.Flags), EventSeq: r.EventSeq, CreatedAt: r.CreatedAt, AckedAt: r.AckedAt}
		if r.ChannelID != nil {
			d.ChannelID = *r.ChannelID
		}
		if r.FromActorID != nil {
			d.FromActorID = *r.FromActorID
		}
		if r.DelegatedBy != nil {
			d.DelegatedBy = *r.DelegatedBy
		}
		out = append(out, d)
	}
	return out, int(total), nil
}

// Ack marks deliveries as handled.
func (s *Store) Ack(ctx context.Context, recipientID ids.ID, idList []ids.ID, at time.Time) (int, error) {
	n, err := s.q(ctx).Ack(ctx, sqlcgen.AckParams{At: at, RecipientID: recipientID, Ids: idList})
	return int(n), err
}

// InsertTerminalEvent stores a terminal completion event in postgres.
func (s *Store) InsertTerminalEvent(ctx context.Context, te delivery.TerminalEvent) error {
	var exitCode *int32
	if te.ExitCode != nil {
		ec := int32(*te.ExitCode)
		exitCode = &ec
	}
	return s.q(ctx).InsertTerminalEvent(ctx, sqlcgen.InsertTerminalEventParams{
		ID:           te.ID,
		WorkspaceID:  te.WorkspaceID,
		AgentID:      te.AgentID,
		ChannelID:    te.ChannelID,
		ThreadRootID: nullID(te.ThreadRootID),
		Name:         te.Name,
		ExitCode:     exitCode,
		DurationMs:   te.DurationMs,
		Tail:         te.Tail,
		CreatedAt:    te.CreatedAt,
	})
}

// TerminalEventsByIDs loads terminal events by their IDs.
func (s *Store) TerminalEventsByIDs(ctx context.Context, eventIDs []ids.ID) ([]delivery.TerminalEvent, error) {
	if len(eventIDs) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).TerminalEventsByIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}
	out := make([]delivery.TerminalEvent, 0, len(rows))
	for _, r := range rows {
		te := delivery.TerminalEvent{
			ID:          r.ID,
			WorkspaceID: r.WorkspaceID,
			AgentID:     r.AgentID,
			ChannelID:   r.ChannelID,
			Name:        r.Name,
			DurationMs:  r.DurationMs,
			Tail:        r.Tail,
			CreatedAt:   r.CreatedAt,
		}
		if r.ThreadRootID != nil {
			te.ThreadRootID = *r.ThreadRootID
		}
		if r.ExitCode != nil {
			ec := int(*r.ExitCode)
			te.ExitCode = &ec
		}
		out = append(out, te)
	}
	return out, nil
}

func nullID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}
