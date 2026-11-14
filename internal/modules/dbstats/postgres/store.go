// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres implements dbstats.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"colonyhall/internal/modules/dbstats"
	"colonyhall/internal/modules/dbstats/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements dbstats.Store.
type Store struct{ DB *db.Postgres }

var _ dbstats.Store = (*Store)(nil)

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func date(t time.Time) pgtype.Date { return pgtype.Date{Time: dbstats.Day(t), Valid: true} }

// Size is pg_database_size of the current database.
func (s *Store) Size(ctx context.Context) (int64, error) { return s.q(ctx).DatabaseSize(ctx) }

// LargestTables lists the current schema's biggest tables first.
func (s *Store) LargestTables(ctx context.Context, limit int) ([]dbstats.TableSize, error) {
	rows, err := s.q(ctx).LargestTables(ctx, int32(min(limit, 1000))) //nolint:gosec // bounded above
	if err != nil {
		return nil, err
	}
	out := make([]dbstats.TableSize, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbstats.TableSize{Name: r.Name, TotalBytes: r.TotalBytes, RowsEstimate: r.RowsEstimate})
	}
	return out, nil
}

// RecordSample keeps the first sample of a day.
func (s *Store) RecordSample(ctx context.Context, x dbstats.Sample) error {
	return s.q(ctx).RecordSizeSample(ctx, sqlcgen.RecordSizeSampleParams{SampledOn: date(x.Day), SizeBytes: x.SizeBytes})
}

// Samples lists the samples from since on, oldest first.
func (s *Store) Samples(ctx context.Context, since time.Time) ([]dbstats.Sample, error) {
	rows, err := s.q(ctx).SizeSamples(ctx, date(since))
	if err != nil {
		return nil, err
	}
	out := make([]dbstats.Sample, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbstats.Sample{Day: dbstats.Day(r.SampledOn.Time), SizeBytes: r.SizeBytes})
	}
	return out, nil
}

// PruneSamples deletes the samples before the day.
func (s *Store) PruneSamples(ctx context.Context, before time.Time) (int64, error) {
	return s.q(ctx).PruneSizeSamples(ctx, date(before))
}

// WorkspaceIDs lists every workspace (system mode).
func (s *Store) WorkspaceIDs(ctx context.Context) ([]ids.ID, error) {
	rows, err := s.q(ctx).WorkspaceIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ids.ID, len(rows))
	for i, r := range rows {
		out[i] = ids.ID(r)
	}
	return out, nil
}

// WorkspaceRowBytes sums the workspace's rows in the large tables.
func (s *Store) WorkspaceRowBytes(ctx context.Context, workspaceID ids.ID) (dbstats.RowBytes, error) {
	r, err := s.q(ctx).WorkspaceRowBytes(ctx, uuid.UUID(workspaceID))
	if err != nil {
		return dbstats.RowBytes{}, err
	}
	return dbstats.RowBytes{
		Chat:  r.Message + r.Event + r.SearchChunk + r.SearchLog + r.Delivery,
		Other: r.AgentRun + r.AuditLog + r.File + r.WikiRevision + r.TerminalEvent,
	}, nil
}

// ChatTablesSize is the chat and search tables' size with indexes.
func (s *Store) ChatTablesSize(ctx context.Context) (int64, error) {
	return s.q(ctx).ChatTablesSize(ctx)
}
