// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres implements approval.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/approval/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements approval.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// Create stores an approval request.
func (s *Store) Create(ctx context.Context, r approval.ApprovalRequest) error {
	var thread *ids.ID
	if r.ThreadRootID != ids.Nil {
		thread = &r.ThreadRootID
	}
	var decidedBy *ids.ID
	if r.DecidedBy != ids.Nil {
		decidedBy = &r.DecidedBy
	}
	return s.q(ctx).CreateApprovalRequest(ctx, sqlcgen.CreateApprovalRequestParams{
		ID:              r.ID,
		WorkspaceID:     r.WorkspaceID,
		AgentID:         r.AgentID,
		ChannelID:       r.ChannelID,
		ThreadRootID:    thread,
		ToolName:        r.ToolName,
		Input:           r.Input,
		State:           r.State,
		DecidedBy:       decidedBy,
		DecisionMessage: r.DecisionMessage,
		CreatedAt:       r.CreatedAt,
		DecidedAt:       r.DecidedAt,
		ExpiresAt:       r.ExpiresAt,
	})
}

// ByID loads an approval request by its ID.
func (s *Store) ByID(ctx context.Context, id ids.ID) (approval.ApprovalRequest, error) {
	r, err := s.q(ctx).ApprovalRequestByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return approval.ApprovalRequest{}, db.ErrNotFound
	}
	if err != nil {
		return approval.ApprovalRequest{}, err
	}
	return toModel(r), nil
}

// Decide updates the state of a pending approval request.
func (s *Store) Decide(ctx context.Context, id ids.ID, state string, decidedBy ids.ID, message string, decidedAt time.Time) error {
	var by *ids.ID
	if decidedBy != ids.Nil {
		by = &decidedBy
	}
	rows, err := s.q(ctx).DecideApprovalRequest(ctx, sqlcgen.DecideApprovalRequestParams{
		ID:              id,
		State:           state,
		DecidedBy:       by,
		DecisionMessage: message,
		DecidedAt:       &decidedAt,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return approval.ErrAlreadyDecided
	}
	return nil
}

// ExpirePendingByWorkspace marks all pending requests in workspace as expired.
func (s *Store) ExpirePendingByWorkspace(ctx context.Context, workspaceID ids.ID, decidedAt time.Time) (int64, error) {
	return s.q(ctx).ExpirePendingByWorkspace(ctx, sqlcgen.ExpirePendingByWorkspaceParams{
		WorkspaceID: workspaceID,
		DecidedAt:   &decidedAt,
	})
}

// ListPendingByWorkspace lists all pending requests for a workspace.
func (s *Store) ListPendingByWorkspace(ctx context.Context, workspaceID ids.ID) ([]approval.ApprovalRequest, error) {
	rows, err := s.q(ctx).ListPendingByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]approval.ApprovalRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, toModel(r))
	}
	return out, nil
}

func toModel(r sqlcgen.ApprovalRequest) approval.ApprovalRequest {
	req := approval.ApprovalRequest{
		ID:              r.ID,
		WorkspaceID:     r.WorkspaceID,
		AgentID:         r.AgentID,
		ChannelID:       r.ChannelID,
		ToolName:        r.ToolName,
		Input:           r.Input,
		State:           r.State,
		DecisionMessage: r.DecisionMessage,
		CreatedAt:       r.CreatedAt,
		DecidedAt:       r.DecidedAt,
		ExpiresAt:       r.ExpiresAt,
	}
	if r.ThreadRootID != nil {
		req.ThreadRootID = *r.ThreadRootID
	}
	if r.DecidedBy != nil {
		req.DecidedBy = *r.DecidedBy
	}
	return req
}
