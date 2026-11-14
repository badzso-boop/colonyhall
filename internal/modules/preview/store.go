// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package preview

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store defines persistence for previews.
type Store interface {
	Create(ctx context.Context, p Preview) error
	ByID(ctx context.Context, id ids.ID) (Preview, error)
	ListByWorkspace(ctx context.Context, workspaceID ids.ID) ([]Preview, error)
	ListByAgent(ctx context.Context, workspaceID, agentID ids.ID) ([]Preview, error)
	ActiveByAgentAndName(ctx context.Context, agentID ids.ID, name string) (Preview, error)
	ActiveByHost(ctx context.Context, hostID ids.ID) ([]Preview, error)
	UpdateStatus(ctx context.Context, id ids.ID, state, url, errStr string, at time.Time) error
	StopActive(ctx context.Context, id ids.ID, at time.Time) error
	StopActiveByAgent(ctx context.Context, agentID ids.ID, at time.Time) error
	StopActiveByWorkspace(ctx context.Context, workspaceID ids.ID, at time.Time) error
}
