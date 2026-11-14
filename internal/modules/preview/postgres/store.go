// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres implements preview.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/preview"
	"colonyhall/internal/modules/preview/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements preview.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Create stores a preview.
func (s *Store) Create(ctx context.Context, p preview.Preview) error {
	return s.q(ctx).CreatePreview(ctx, sqlcgen.CreatePreviewParams{
		ID:           p.ID,
		WorkspaceID:  p.WorkspaceID,
		AgentID:      p.AgentID,
		HostID:       p.HostID,
		ChannelID:    p.ChannelID,
		ThreadRootID: p.ThreadRootID,
		Name:         p.Name,
		ComposePath:  p.ComposePath,
		State:        p.State,
		Url:          p.URL,
		Error:        p.Error,
		ExpiresAt:    p.ExpiresAt,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
	})
}

// ByID loads a preview by its ID.
func (s *Store) ByID(ctx context.Context, id ids.ID) (preview.Preview, error) {
	r, err := s.q(ctx).GetPreviewByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return preview.Preview{}, db.ErrNotFound
	}
	if err != nil {
		return preview.Preview{}, err
	}
	return toModel(r), nil
}

// ListByWorkspace lists all previews for a workspace sorted by created_at DESC.
func (s *Store) ListByWorkspace(ctx context.Context, workspaceID ids.ID) ([]preview.Preview, error) {
	rows, err := s.q(ctx).ListPreviewsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]preview.Preview, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

// ListByAgent lists all previews for an agent sorted by created_at DESC.
func (s *Store) ListByAgent(ctx context.Context, workspaceID, agentID ids.ID) ([]preview.Preview, error) {
	rows, err := s.q(ctx).ListPreviewsByAgent(ctx, sqlcgen.ListPreviewsByAgentParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]preview.Preview, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

// ActiveByAgentAndName returns the active (requested or running) preview for an agent by name.
func (s *Store) ActiveByAgentAndName(ctx context.Context, agentID ids.ID, name string) (preview.Preview, error) {
	r, err := s.q(ctx).GetActivePreviewByAgentAndName(ctx, sqlcgen.GetActivePreviewByAgentAndNameParams{
		AgentID: agentID,
		Name:    name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return preview.Preview{}, db.ErrNotFound
	}
	if err != nil {
		return preview.Preview{}, err
	}
	return toModel(r), nil
}

// ActiveByHost returns all active (requested or running) previews for a host sorted by created_at ASC.
func (s *Store) ActiveByHost(ctx context.Context, hostID ids.ID) ([]preview.Preview, error) {
	rows, err := s.q(ctx).ListActivePreviewsByHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := make([]preview.Preview, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

// UpdateStatus updates status, URL, error and updated_at.
func (s *Store) UpdateStatus(ctx context.Context, id ids.ID, state, url, errStr string, at time.Time) error {
	_, err := s.q(ctx).UpdatePreviewStatus(ctx, sqlcgen.UpdatePreviewStatusParams{
		ID:        id,
		State:     state,
		Url:       url,
		Error:     errStr,
		UpdatedAt: at,
	})
	return err
}

// StopActive marks an active preview as stopped.
func (s *Store) StopActive(ctx context.Context, id ids.ID, at time.Time) error {
	_, err := s.q(ctx).StopActivePreview(ctx, sqlcgen.StopActivePreviewParams{
		ID:        id,
		UpdatedAt: at,
	})
	return err
}

// StopActiveByAgent stops all active previews for an agent.
func (s *Store) StopActiveByAgent(ctx context.Context, agentID ids.ID, at time.Time) error {
	_, err := s.q(ctx).StopActivePreviewsByAgent(ctx, sqlcgen.StopActivePreviewsByAgentParams{
		AgentID:   agentID,
		UpdatedAt: at,
	})
	return err
}

// StopActiveByWorkspace stops all active previews in a workspace.
func (s *Store) StopActiveByWorkspace(ctx context.Context, workspaceID ids.ID, at time.Time) error {
	_, err := s.q(ctx).StopActivePreviewsByWorkspace(ctx, sqlcgen.StopActivePreviewsByWorkspaceParams{
		WorkspaceID: workspaceID,
		UpdatedAt:   at,
	})
	return err
}

func toModel(r sqlcgen.Preview) preview.Preview {
	return preview.Preview{
		ID:           r.ID,
		WorkspaceID:  r.WorkspaceID,
		AgentID:      r.AgentID,
		HostID:       r.HostID,
		ChannelID:    r.ChannelID,
		ThreadRootID: r.ThreadRootID,
		Name:         r.Name,
		ComposePath:  r.ComposePath,
		State:        r.State,
		URL:          r.Url,
		Error:        r.Error,
		ExpiresAt:    r.ExpiresAt,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}
