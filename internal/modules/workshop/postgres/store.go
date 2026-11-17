// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package postgres is the Postgres Workshop store.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/modules/workshop/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements workshop.Store on PostgreSQL.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

func optID(id ids.ID) *uuid.UUID {
	if id == ids.Nil {
		return nil
	}
	u := uuid.UUID(id)
	return &u
}

func fromOpt(u *uuid.UUID) ids.ID {
	if u == nil {
		return ids.Nil
	}
	return *u
}

func rows(n int64, err error) error {
	if err != nil {
		return err
	}
	if n == 0 {
		return db.ErrNotFound
	}
	return nil
}

func connector(r sqlcgen.Connector) (workshop.Connector, error) {
	c := workshop.Connector{ID: r.ID, WorkspaceID: r.WorkspaceID, Slug: r.Slug, Title: r.Title, Kind: r.Kind,
		BaseURL: r.BaseUrl, Auth: r.Auth, HeaderName: r.HeaderName, SecretID: fromOpt(r.SecretID),
		ApprovalOff: r.ApprovalOff, ToolsCachedAt: r.ToolsCachedAt, State: r.State, LastError: r.LastError,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt}
	if err := json.Unmarshal(r.RiskOverrides, &c.RiskOverrides); err != nil {
		return c, err
	}
	if c.RiskOverrides == nil {
		c.RiskOverrides = map[string]string{}
	}
	if err := json.Unmarshal(r.ToolsCache, &c.Tools); err != nil {
		return c, err
	}
	return c, nil
}

func overrides(m map[string]string) []byte {
	if m == nil {
		m = map[string]string{}
	}
	b, _ := json.Marshal(m)
	return b
}

func tools(t []workshop.ToolInfo) []byte {
	if t == nil {
		t = []workshop.ToolInfo{}
	}
	b, _ := json.Marshal(t)
	return b
}

// InsertConnector implements workshop.Store.
func (s *Store) InsertConnector(ctx context.Context, c workshop.Connector) error {
	err := s.q(ctx).InsertConnector(ctx, sqlcgen.InsertConnectorParams{ID: c.ID, WorkspaceID: c.WorkspaceID,
		Slug: c.Slug, Title: c.Title, Kind: c.Kind, BaseUrl: c.BaseURL, Auth: c.Auth, HeaderName: c.HeaderName,
		SecretID: optID(c.SecretID), ApprovalOff: c.ApprovalOff, RiskOverrides: overrides(c.RiskOverrides),
		ToolsCache: tools(c.Tools), State: c.State, LastError: c.LastError, CreatedBy: c.CreatedBy,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// UpdateConnector implements workshop.Store.
func (s *Store) UpdateConnector(ctx context.Context, c workshop.Connector) error {
	return rows(s.q(ctx).UpdateConnector(ctx, sqlcgen.UpdateConnectorParams{WorkspaceID: c.WorkspaceID, ID: c.ID,
		Title: c.Title, BaseUrl: c.BaseURL, Auth: c.Auth, HeaderName: c.HeaderName, SecretID: optID(c.SecretID),
		ApprovalOff: c.ApprovalOff, RiskOverrides: overrides(c.RiskOverrides), UpdatedAt: c.UpdatedAt}))
}

// SetConnectorTools implements workshop.Store.
func (s *Store) SetConnectorTools(ctx context.Context, workspaceID, id ids.ID, t []workshop.ToolInfo, at time.Time, state, lastError string) error {
	return rows(s.q(ctx).SetConnectorTools(ctx, sqlcgen.SetConnectorToolsParams{WorkspaceID: workspaceID, ID: id,
		ToolsCache: tools(t), ToolsCachedAt: &at, State: state, LastError: lastError}))
}

// GetConnector implements workshop.Store.
func (s *Store) GetConnector(ctx context.Context, workspaceID, id ids.ID) (workshop.Connector, error) {
	r, err := s.q(ctx).GetConnector(ctx, sqlcgen.GetConnectorParams{WorkspaceID: workspaceID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return workshop.Connector{}, db.ErrNotFound
	}
	if err != nil {
		return workshop.Connector{}, err
	}
	return connector(r)
}

// ListConnectors implements workshop.Store.
func (s *Store) ListConnectors(ctx context.Context, workspaceID ids.ID) ([]workshop.Connector, error) {
	rs, err := s.q(ctx).ListConnectors(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]workshop.Connector, 0, len(rs))
	for _, r := range rs {
		c, err := connector(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// DeleteConnector implements workshop.Store.
func (s *Store) DeleteConnector(ctx context.Context, workspaceID, id ids.ID, at time.Time) error {
	return rows(s.q(ctx).DeleteConnector(ctx, sqlcgen.DeleteConnectorParams{WorkspaceID: workspaceID, ID: id, DeletedAt: &at}))
}

// InsertGrant implements workshop.Store.
func (s *Store) InsertGrant(ctx context.Context, g workshop.Grant) error {
	err := s.q(ctx).InsertGrant(ctx, sqlcgen.InsertGrantParams{ID: g.ID, WorkspaceID: g.WorkspaceID, AgentID: g.AgentID,
		TargetKind: g.TargetKind, TargetID: optID(g.TargetID), GrantedBy: g.GrantedBy, CreatedAt: g.CreatedAt})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// RevokeGrant implements workshop.Store.
func (s *Store) RevokeGrant(ctx context.Context, workspaceID, id ids.ID, at time.Time) error {
	return rows(s.q(ctx).RevokeGrant(ctx, sqlcgen.RevokeGrantParams{WorkspaceID: workspaceID, ID: id, RevokedAt: &at}))
}

// RevokeGrantsForTarget implements workshop.Store.
func (s *Store) RevokeGrantsForTarget(ctx context.Context, workspaceID ids.ID, kind string, targetID ids.ID, at time.Time) error {
	return s.q(ctx).RevokeGrantsForTarget(ctx, sqlcgen.RevokeGrantsForTargetParams{WorkspaceID: workspaceID,
		TargetKind: kind, TargetID: optID(targetID), RevokedAt: &at})
}

func grant(r sqlcgen.AgentToolGrant) workshop.Grant {
	return workshop.Grant{ID: r.ID, WorkspaceID: r.WorkspaceID, AgentID: r.AgentID, TargetKind: r.TargetKind,
		TargetID: fromOpt(r.TargetID), GrantedBy: r.GrantedBy, CreatedAt: r.CreatedAt, RevokedAt: r.RevokedAt}
}

// ListGrants implements workshop.Store.
func (s *Store) ListGrants(ctx context.Context, workspaceID ids.ID) ([]workshop.Grant, error) {
	rs, err := s.q(ctx).ListGrants(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]workshop.Grant, len(rs))
	for i, r := range rs {
		out[i] = grant(r)
	}
	return out, nil
}

// GrantsForAgent implements workshop.Store.
func (s *Store) GrantsForAgent(ctx context.Context, workspaceID, agentID ids.ID) ([]workshop.Grant, error) {
	rs, err := s.q(ctx).GrantsForAgent(ctx, sqlcgen.GrantsForAgentParams{WorkspaceID: workspaceID, AgentID: agentID})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.Grant, len(rs))
	for i, r := range rs {
		out[i] = grant(r)
	}
	return out, nil
}

// InsertToolCall implements workshop.Store.
func (s *Store) InsertToolCall(ctx context.Context, c workshop.ToolCall) error {
	return s.q(ctx).InsertToolCall(ctx, sqlcgen.InsertToolCallParams{ID: c.ID, WorkspaceID: c.WorkspaceID,
		ActorID: c.ActorID, TargetKind: c.TargetKind, TargetID: c.TargetID, Tool: c.Tool, Risk: c.Risk,
		ApprovalID: optID(c.ApprovalID), StartedAt: c.StartedAt, DurationMs: int32(c.DurationMS),
		InBytes: int32(c.InBytes), OutBytes: int32(c.OutBytes), Outcome: c.Outcome, Error: c.Error})
}

// ListToolCalls implements workshop.Store.
func (s *Store) ListToolCalls(ctx context.Context, workspaceID ids.ID, targetID *ids.ID, before *time.Time, limit int) ([]workshop.ToolCall, error) {
	var tid *uuid.UUID
	if targetID != nil {
		u := uuid.UUID(*targetID)
		tid = &u
	}
	rs, err := s.q(ctx).ListToolCalls(ctx, sqlcgen.ListToolCallsParams{WorkspaceID: workspaceID, Limit: int32(limit),
		TargetID: tid, Before: before})
	if err != nil {
		return nil, err
	}
	out := make([]workshop.ToolCall, len(rs))
	for i, r := range rs {
		out[i] = workshop.ToolCall{ID: r.ID, WorkspaceID: r.WorkspaceID, ActorID: r.ActorID, TargetKind: r.TargetKind,
			TargetID: r.TargetID, Tool: r.Tool, Risk: r.Risk, ApprovalID: fromOpt(r.ApprovalID), StartedAt: r.StartedAt,
			DurationMS: int(r.DurationMs), InBytes: int(r.InBytes), OutBytes: int(r.OutBytes), Outcome: r.Outcome, Error: r.Error}
	}
	return out, nil
}
