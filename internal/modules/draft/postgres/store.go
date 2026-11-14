// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the PostgreSQL draft store.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/draft"
	"colonyhall/internal/modules/draft/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements draft.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func thread(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func fromRow(r sqlcgen.Draft) draft.Draft {
	d := draft.Draft{ID: r.ID, WorkspaceID: r.WorkspaceID, ActorID: r.ActorID, ChannelID: r.ChannelID,
		Body: r.Body, UpdatedAt: r.UpdatedAt, FileIDs: make([]ids.ID, len(r.FileIds))}
	if r.ThreadRootID != nil {
		d.ThreadRootID = *r.ThreadRootID
	}
	copy(d.FileIDs, r.FileIds)
	return d
}

// Upsert implements draft.Store.
func (s *Store) Upsert(ctx context.Context, d draft.Draft) (draft.Draft, error) {
	files := make([]uuid.UUID, len(d.FileIDs))
	copy(files, d.FileIDs)
	r, err := s.q(ctx).UpsertDraft(ctx, sqlcgen.UpsertDraftParams{ID: d.ID, WorkspaceID: d.WorkspaceID, ActorID: d.ActorID,
		ChannelID: d.ChannelID, ThreadRootID: thread(d.ThreadRootID), Body: d.Body, FileIds: files, UpdatedAt: d.UpdatedAt})
	if err != nil {
		return draft.Draft{}, err
	}
	return fromRow(r), nil
}

// Get implements draft.Store.
func (s *Store) Get(ctx context.Context, actorID, channelID, threadRootID ids.ID) (draft.Draft, error) {
	r, err := s.q(ctx).GetDraft(ctx, sqlcgen.GetDraftParams{ActorID: actorID, ChannelID: channelID, ThreadRootID: thread(threadRootID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return draft.Draft{}, db.ErrNotFound
	}
	if err != nil {
		return draft.Draft{}, err
	}
	return fromRow(r), nil
}

// Delete implements draft.Store.
func (s *Store) Delete(ctx context.Context, actorID, channelID, threadRootID ids.ID, notAfter *time.Time) (bool, error) {
	n, err := s.q(ctx).DeleteDraft(ctx, sqlcgen.DeleteDraftParams{ActorID: actorID, ChannelID: channelID,
		ThreadRootID: thread(threadRootID), NotAfter: notAfter})
	return n > 0, err
}

// List implements draft.Store.
func (s *Store) List(ctx context.Context, workspaceID, actorID ids.ID, limit int) ([]draft.Draft, error) {
	rows, err := s.q(ctx).ListDrafts(ctx, sqlcgen.ListDraftsParams{WorkspaceID: workspaceID, ActorID: actorID, Limit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]draft.Draft, len(rows))
	for i, r := range rows {
		out[i] = fromRow(r)
	}
	return out, nil
}
