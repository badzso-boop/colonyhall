// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists the Workshop. Every call runs in the workspace's scope;
// "not found" is db.ErrNotFound.
type Store interface {
	InsertConnector(ctx context.Context, c Connector) error
	// UpdateConnector writes title, base URL, auth, header, secret,
	// approval_off and the risk overrides.
	UpdateConnector(ctx context.Context, c Connector) error
	SetConnectorTools(ctx context.Context, workspaceID, id ids.ID, tools []ToolInfo, at time.Time, state, lastError string) error
	GetConnector(ctx context.Context, workspaceID, id ids.ID) (Connector, error)
	ListConnectors(ctx context.Context, workspaceID ids.ID) ([]Connector, error)
	DeleteConnector(ctx context.Context, workspaceID, id ids.ID, at time.Time) error

	InsertGrant(ctx context.Context, g Grant) error
	RevokeGrant(ctx context.Context, workspaceID, id ids.ID, at time.Time) error
	RevokeGrantsForTarget(ctx context.Context, workspaceID ids.ID, kind string, targetID ids.ID, at time.Time) error
	ListGrants(ctx context.Context, workspaceID ids.ID) ([]Grant, error)
	GrantsForAgent(ctx context.Context, workspaceID, agentID ids.ID) ([]Grant, error)

	InsertToolCall(ctx context.Context, c ToolCall) error
	// ListToolCalls returns the newest calls first, before the given time
	// (nil: from now), optionally of one target.
	ListToolCalls(ctx context.Context, workspaceID ids.ID, targetID *ids.ID, before *time.Time, limit int) ([]ToolCall, error)
}
