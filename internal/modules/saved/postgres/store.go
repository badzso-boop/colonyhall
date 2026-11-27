// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the PostgreSQL saved-item store.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/saved"
	"colonyhall/internal/modules/saved/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements saved.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func fromRow(r sqlcgen.SavedItem) saved.Item {
	it := saved.Item{ID: r.ID, WorkspaceID: r.WorkspaceID, ActorID: r.ActorID, Note: r.Note, RemindAt: r.RemindAt,
		RemindedAt: r.RemindedAt, DoneAt: r.DoneAt, CreatedAt: r.CreatedAt}
	if r.MessageID != nil {
		it.MessageID = *r.MessageID
	}
	if r.ChannelID != nil {
		it.ChannelID = *r.ChannelID
	}
	return it
}

func one(r sqlcgen.SavedItem, err error) (saved.Item, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return saved.Item{}, db.ErrNotFound
	}
	if err != nil {
		return saved.Item{}, err
	}
	return fromRow(r), nil
}

func many(rows []sqlcgen.SavedItem, err error) ([]saved.Item, error) {
	if err != nil {
		return nil, err
	}
	out := make([]saved.Item, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}

// Insert implements saved.Store.
func (s *Store) Insert(ctx context.Context, it saved.Item) error {
	err := s.q(ctx).InsertSaved(ctx, sqlcgen.InsertSavedParams{ID: it.ID, WorkspaceID: it.WorkspaceID, ActorID: it.ActorID,
		MessageID: ids.NilAsNull(it.MessageID), ChannelID: ids.NilAsNull(it.ChannelID), Note: it.Note, RemindAt: it.RemindAt,
		RemindedAt: it.RemindedAt, DoneAt: it.DoneAt, CreatedAt: it.CreatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// Update implements saved.Store.
func (s *Store) Update(ctx context.Context, it saved.Item) error {
	return s.q(ctx).UpdateSaved(ctx, sqlcgen.UpdateSavedParams{ID: it.ID, Note: it.Note, RemindAt: it.RemindAt,
		RemindedAt: it.RemindedAt, DoneAt: it.DoneAt})
}

// Get implements saved.Store.
func (s *Store) Get(ctx context.Context, id ids.ID) (saved.Item, error) {
	return one(s.q(ctx).GetSaved(ctx, id))
}

// ByMessage implements saved.Store.
func (s *Store) ByMessage(ctx context.Context, actorID, messageID ids.ID) (saved.Item, error) {
	return one(s.q(ctx).SavedByMessage(ctx, sqlcgen.SavedByMessageParams{ActorID: actorID, MessageID: ids.NilAsNull(messageID)}))
}

// Delete implements saved.Store.
func (s *Store) Delete(ctx context.Context, id ids.ID) error { return s.q(ctx).DeleteSaved(ctx, id) }

// List implements saved.Store.
func (s *Store) List(ctx context.Context, workspaceID, actorID ids.ID, f saved.Filter, after *saved.Cursor, limit int) ([]saved.Item, error) {
	p := sqlcgen.ListSavedParams{WorkspaceID: workspaceID, ActorID: actorID, Filter: string(f), PageSize: int32(limit)}
	if after != nil {
		p.BeforeAt, p.BeforeID = &after.At, ids.NilAsNull(after.ID)
	}
	return many(s.q(ctx).ListSaved(ctx, p))
}

// Count implements saved.Store.
func (s *Store) Count(ctx context.Context, workspaceID, actorID ids.ID) (int, error) {
	n, err := s.q(ctx).CountSaved(ctx, sqlcgen.CountSavedParams{WorkspaceID: workspaceID, ActorID: actorID})
	return int(n), err
}

// Due implements saved.Store.
func (s *Store) Due(ctx context.Context, now time.Time, limit int) ([]saved.Item, error) {
	return many(s.q(ctx).DueReminders(ctx, sqlcgen.DueRemindersParams{Now: now, PageSize: int32(limit)}))
}

// ClaimReminder implements saved.Store.
func (s *Store) ClaimReminder(ctx context.Context, id ids.ID, at time.Time) (bool, error) {
	n, err := s.q(ctx).ClaimReminder(ctx, sqlcgen.ClaimReminderParams{At: at, ID: id})
	return n > 0, err
}
