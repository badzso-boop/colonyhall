// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the PostgreSQL activity store.
package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"colonyhall/internal/modules/activity"
	"colonyhall/internal/modules/activity/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements activity.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func val(p *uuid.UUID) ids.ID {
	if p == nil {
		return ids.Nil
	}
	return *p
}

// Insert implements activity.Store.
func (s *Store) Insert(ctx context.Context, it activity.Item) (bool, error) {
	n, err := s.q(ctx).InsertActivity(ctx, sqlcgen.InsertActivityParams{ID: it.ID, WorkspaceID: it.WorkspaceID, RecipientID: it.RecipientID,
		Kind: string(it.Kind), ChannelID: ids.NilAsNull(it.ChannelID), MessageID: ids.NilAsNull(it.MessageID), ThreadRootID: ids.NilAsNull(it.ThreadRootID),
		ActorID: ids.NilAsNull(it.ActorID), RefID: ids.NilAsNull(it.RefID), Emoji: it.Emoji, SourceSeq: it.SourceSeq, CreatedAt: it.CreatedAt})
	return n > 0, err
}

// List implements activity.Store.
func (s *Store) List(ctx context.Context, workspaceID, recipientID ids.ID, q activity.Query) ([]activity.Item, error) {
	p := sqlcgen.ListActivityParams{WorkspaceID: workspaceID, RecipientID: recipientID, Kind: string(q.Kind), UnreadOnly: q.UnreadOnly, PageSize: int32(q.Limit)}
	if q.After != nil {
		p.BeforeAt, p.BeforeID = &q.After.At, ids.NilAsNull(q.After.ID)
	}
	rows, err := s.q(ctx).ListActivity(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]activity.Item, len(rows))
	for i, r := range rows {
		out[i] = activity.Item{ID: r.ID, WorkspaceID: r.WorkspaceID, RecipientID: r.RecipientID, Kind: activity.Kind(r.Kind),
			ChannelID: val(r.ChannelID), MessageID: val(r.MessageID), ThreadRootID: val(r.ThreadRootID), ActorID: val(r.ActorID),
			RefID: val(r.RefID), Emoji: r.Emoji, SourceSeq: r.SourceSeq, CreatedAt: r.CreatedAt, ReadAt: r.ReadAt}
	}
	return out, nil
}

// CountUnread implements activity.Store.
func (s *Store) CountUnread(ctx context.Context, workspaceID, recipientID ids.ID) (int, error) {
	n, err := s.q(ctx).CountUnreadActivity(ctx, sqlcgen.CountUnreadActivityParams{WorkspaceID: workspaceID, RecipientID: recipientID})
	return int(n), err
}

// MarkRead implements activity.Store.
func (s *Store) MarkRead(ctx context.Context, recipientID ids.ID, itemIDs []ids.ID, channelID ids.ID, at time.Time) (int, error) {
	list := make([]uuid.UUID, len(itemIDs))
	copy(list, itemIDs)
	n, err := s.q(ctx).MarkActivityRead(ctx, sqlcgen.MarkActivityReadParams{At: at, RecipientID: recipientID, ItemIds: list, ChannelID: ids.NilAsNull(channelID)})
	return int(n), err
}
