// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/agentrules"
	"colonyhall/internal/modules/agentrules/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements agentrules.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Insert stores a new agent rules version.
func (s *Store) Insert(ctx context.Context, r agentrules.Rules) error {
	return s.q(ctx).InsertRules(ctx, sqlcgen.InsertRulesParams{
		WorkspaceID: r.WorkspaceID,
		Version:     int32(r.Version),
		Text:        r.Text,
		CreatedBy:   r.CreatedBy,
		CreatedAt:   r.CreatedAt,
	})
}

// GetLatest returns the highest version of rules for the workspace.
func (s *Store) GetLatest(ctx context.Context, workspaceID ids.ID) (agentrules.Rules, error) {
	row, err := s.q(ctx).GetLatestRules(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return agentrules.Rules{}, db.ErrNotFound
	}
	if err != nil {
		return agentrules.Rules{}, err
	}
	return agentrules.Rules{
		WorkspaceID: row.WorkspaceID,
		Version:     int(row.Version),
		Text:        row.Text,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// GetVersion returns a specific version of rules for the workspace.
func (s *Store) GetVersion(ctx context.Context, workspaceID ids.ID, version int) (agentrules.Rules, error) {
	row, err := s.q(ctx).GetRulesVersion(ctx, sqlcgen.GetRulesVersionParams{
		WorkspaceID: workspaceID,
		Version:     int32(version),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return agentrules.Rules{}, db.ErrNotFound
	}
	if err != nil {
		return agentrules.Rules{}, err
	}
	return agentrules.Rules{
		WorkspaceID: row.WorkspaceID,
		Version:     int(row.Version),
		Text:        row.Text,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// ListHistory returns version history for the workspace ordered by version descending.
func (s *Store) ListHistory(ctx context.Context, workspaceID ids.ID) ([]agentrules.RulesMeta, error) {
	rows, err := s.q(ctx).ListRulesHistory(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	meta := make([]agentrules.RulesMeta, len(rows))
	for i, r := range rows {
		meta[i] = agentrules.RulesMeta{
			Version:   int(r.Version),
			CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt,
		}
	}
	return meta, nil
}
