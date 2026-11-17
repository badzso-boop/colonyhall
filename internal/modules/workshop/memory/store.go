// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// Package memory is the in-memory Workshop store (tests, run-mem).
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store keeps the Workshop in memory.
type Store struct {
	mu         sync.Mutex
	connectors map[ids.ID]workshop.Connector
	grants     map[ids.ID]workshop.Grant
	calls      []workshop.ToolCall
}

// New returns an empty store.
func New() *Store {
	return &Store{connectors: map[ids.ID]workshop.Connector{}, grants: map[ids.ID]workshop.Grant{}}
}

func copyConnector(c workshop.Connector) workshop.Connector {
	o := make(map[string]string, len(c.RiskOverrides))
	for k, v := range c.RiskOverrides {
		o[k] = v
	}
	c.RiskOverrides = o
	c.Tools = append([]workshop.ToolInfo(nil), c.Tools...)
	return c
}

// InsertConnector implements workshop.Store.
func (s *Store) InsertConnector(_ context.Context, c workshop.Connector) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.connectors {
		if x.WorkspaceID == c.WorkspaceID && x.Slug == c.Slug && x.DeletedAt == nil {
			return db.ErrConflict
		}
	}
	s.connectors[c.ID] = copyConnector(c)
	return nil
}

func (s *Store) live(workspaceID, id ids.ID) (workshop.Connector, bool) {
	c, ok := s.connectors[id]
	return c, ok && c.WorkspaceID == workspaceID && c.DeletedAt == nil
}

// UpdateConnector implements workshop.Store.
func (s *Store) UpdateConnector(_ context.Context, c workshop.Connector) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.live(c.WorkspaceID, c.ID)
	if !ok {
		return db.ErrNotFound
	}
	cur.Title, cur.BaseURL, cur.Auth, cur.HeaderName, cur.SecretID = c.Title, c.BaseURL, c.Auth, c.HeaderName, c.SecretID
	cur.ApprovalOff, cur.RiskOverrides, cur.UpdatedAt = c.ApprovalOff, c.RiskOverrides, c.UpdatedAt
	s.connectors[c.ID] = copyConnector(cur)
	return nil
}

// SetConnectorTools implements workshop.Store.
func (s *Store) SetConnectorTools(_ context.Context, workspaceID, id ids.ID, tools []workshop.ToolInfo, at time.Time, state, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.live(workspaceID, id)
	if !ok {
		return db.ErrNotFound
	}
	cur.Tools, cur.ToolsCachedAt, cur.State, cur.LastError, cur.UpdatedAt = tools, &at, state, lastError, at
	s.connectors[id] = copyConnector(cur)
	return nil
}

// GetConnector implements workshop.Store.
func (s *Store) GetConnector(_ context.Context, workspaceID, id ids.ID) (workshop.Connector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.live(workspaceID, id)
	if !ok {
		return workshop.Connector{}, db.ErrNotFound
	}
	return copyConnector(c), nil
}

// ListConnectors implements workshop.Store.
func (s *Store) ListConnectors(_ context.Context, workspaceID ids.ID) ([]workshop.Connector, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.Connector
	for _, c := range s.connectors {
		if c.WorkspaceID == workspaceID && c.DeletedAt == nil {
			out = append(out, copyConnector(c))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// DeleteConnector implements workshop.Store.
func (s *Store) DeleteConnector(_ context.Context, workspaceID, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.live(workspaceID, id)
	if !ok {
		return db.ErrNotFound
	}
	c.DeletedAt, c.UpdatedAt = &at, at
	s.connectors[id] = c
	return nil
}

// InsertGrant implements workshop.Store.
func (s *Store) InsertGrant(_ context.Context, g workshop.Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, x := range s.grants {
		if x.WorkspaceID == g.WorkspaceID && x.AgentID == g.AgentID && x.TargetKind == g.TargetKind &&
			x.TargetID == g.TargetID && x.RevokedAt == nil {
			return db.ErrConflict
		}
	}
	s.grants[g.ID] = g
	return nil
}

// RevokeGrant implements workshop.Store.
func (s *Store) RevokeGrant(_ context.Context, workspaceID, id ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.grants[id]
	if !ok || g.WorkspaceID != workspaceID || g.RevokedAt != nil {
		return db.ErrNotFound
	}
	g.RevokedAt = &at
	s.grants[id] = g
	return nil
}

// RevokeGrantsForTarget implements workshop.Store.
func (s *Store) RevokeGrantsForTarget(_ context.Context, workspaceID ids.ID, kind string, targetID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, g := range s.grants {
		if g.WorkspaceID == workspaceID && g.TargetKind == kind && g.TargetID == targetID && g.RevokedAt == nil {
			g.RevokedAt = &at
			s.grants[id] = g
		}
	}
	return nil
}

func (s *Store) grantList(keep func(workshop.Grant) bool) []workshop.Grant {
	var out []workshop.Grant
	for _, g := range s.grants {
		if g.RevokedAt == nil && keep(g) {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out
}

// ListGrants implements workshop.Store.
func (s *Store) ListGrants(_ context.Context, workspaceID ids.ID) ([]workshop.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grantList(func(g workshop.Grant) bool { return g.WorkspaceID == workspaceID }), nil
}

// GrantsForAgent implements workshop.Store.
func (s *Store) GrantsForAgent(_ context.Context, workspaceID, agentID ids.ID) ([]workshop.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.grantList(func(g workshop.Grant) bool { return g.WorkspaceID == workspaceID && g.AgentID == agentID }), nil
}

// InsertToolCall implements workshop.Store.
func (s *Store) InsertToolCall(_ context.Context, c workshop.ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, c)
	return nil
}

// ListToolCalls implements workshop.Store.
func (s *Store) ListToolCalls(_ context.Context, workspaceID ids.ID, targetID *ids.ID, before *time.Time, limit int) ([]workshop.ToolCall, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []workshop.ToolCall
	for _, c := range s.calls {
		if c.WorkspaceID != workspaceID || (targetID != nil && c.TargetID != *targetID) ||
			(before != nil && !c.StartedAt.Before(*before)) {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.After(out[j].StartedAt)
		}
		return out[i].ID.String() > out[j].ID.String()
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
