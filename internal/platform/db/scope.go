// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package db

import (
	"context"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/platform/db/opscope"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/platform/ids"
)

// Scope is whom a database call works for: one workspace, or every
// workspace (system mode, background code only). Row-level security reads
// it from the connection settings colony.workspace_id and colony.system;
// without either, a query sees no rows.
type Scope struct {
	Workspace ids.ID
	System    bool
	// Operator: the platform operator's scope (round 51); it sees the
	// platform tables and no workspace.
	Operator bool
	// ReadOnly: every transaction on the connection is read-only (round
	// 51b, the operator's break-glass session): a write is refused by
	// Postgres itself, whatever the code above tries.
	ReadOnly bool
}

type readOnlyKey struct{}

// ReadOnly marks ctx read-only: the connections it gets refuse writes
// (default_transaction_read_only). It survives WithWorkspace.
func ReadOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, readOnlyKey{}, true)
}

// IsReadOnly reports whether ctx is read-only.
func IsReadOnly(ctx context.Context) bool {
	v, _ := ctx.Value(readOnlyKey{}).(bool)
	return v
}

type workspaceKey struct{}

// WithWorkspace narrows ctx to one workspace and switches system mode off.
// Request paths call it once the caller is known; background loops call it
// for each item they handle.
func WithWorkspace(ctx context.Context, id ids.ID) context.Context {
	return context.WithValue(opscope.Off(system.Off(ctx)), workspaceKey{}, id)
}

// ScopeOf returns ctx's scope.
func ScopeOf(ctx context.Context) Scope {
	id, _ := ctx.Value(workspaceKey{}).(ids.ID)
	ro := IsReadOnly(ctx)
	if system.Is(ctx) {
		return Scope{System: true, ReadOnly: ro}
	}
	if opscope.Is(ctx) {
		return Scope{Operator: true, ReadOnly: ro}
	}
	return Scope{Workspace: id, ReadOnly: ro}
}

// settings returns the values of colony.workspace_id, colony.system and
// colony.operator.
func (s Scope) settings() (ws, sys, op string) {
	if s.Workspace != ids.Nil {
		ws = s.Workspace.String()
	}
	if s.System {
		sys = "on"
	}
	if s.Operator {
		op = "on"
	}
	return ws, sys, op
}

const scopeDataKey = "colony.scope"

// applyScope sets the connection's scope from ctx every time the pool hands
// the connection out (pgxpool PrepareConn), for transactions and single
// queries alike. Every acquisition overwrites both settings, empty when ctx
// has no scope, so nothing carries over from the previous user; a
// connection that already has the right values gets no extra round trip.
func applyScope(ctx context.Context, c *pgx.Conn) (bool, error) {
	want := ScopeOf(ctx)
	data := c.PgConn().CustomData()
	if have, ok := data[scopeDataKey].(Scope); ok && have == want {
		return true, nil
	}
	ws, sys, op := want.settings()
	ro := "off"
	if want.ReadOnly {
		ro = "on"
	}
	if _, err := c.Exec(ctx, "SELECT set_config('colony.workspace_id', $1, false), set_config('colony.system', $2, false), set_config('colony.operator', $3, false), set_config('default_transaction_read_only', $4, false)", ws, sys, op, ro); err != nil {
		delete(data, scopeDataKey)
		return false, err // destroy the connection: its settings are unknown
	}
	data[scopeDataKey] = want
	return true, nil
}
