// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package server

import (
	"context"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/ids"
)

// breakGlass connects the operator's break-glass access (round 51b) to the
// identity module, which owns the access rows, the operator actor and the
// sessions; each call narrows to the one workspace it concerns.
type breakGlass struct {
	identity *identity.Service
}

func (b breakGlass) OpenBreakGlass(ctx context.Context, workspaceID ids.ID, o operator.Operator, reason string, expiresAt time.Time, devMode bool) (operator.BreakGlassLink, error) {
	// The customer sees the operator's display name, never the login
	// handle: with the handle a customer could try (and lock out) the
	// operator's login where admin.<domain> has no allowlist.
	name := o.DisplayName
	if name == "" {
		name = "operator"
	}
	x, token, err := b.identity.OpenBreakGlass(ctx, identity.OpenBreakGlassInput{WorkspaceID: workspaceID, OperatorID: o.ID,
		OperatorHandle: name, Reason: reason, ExpiresAt: expiresAt, DevMode: devMode})
	if err != nil {
		return operator.BreakGlassLink{}, err
	}
	return operator.BreakGlassLink{BreakGlass: toOperatorBreakGlass(x, b.identity.Clock.Now()), Token: token, HandoffExpiresAt: x.HandoffExpiresAt}, nil
}

func (b breakGlass) ListBreakGlass(ctx context.Context, workspaceID ids.ID) ([]operator.BreakGlass, error) {
	list, err := b.identity.ListBreakGlassFor(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	now := b.identity.Clock.Now()
	out := make([]operator.BreakGlass, len(list))
	for i, x := range list {
		out[i] = toOperatorBreakGlass(x, now)
	}
	return out, nil
}

func (b breakGlass) CloseBreakGlass(ctx context.Context, workspaceID, id ids.ID) error {
	return b.identity.CloseBreakGlass(ctx, nil, workspaceID, id)
}

func toOperatorBreakGlass(x identity.BreakGlass, now time.Time) operator.BreakGlass {
	return operator.BreakGlass{ID: x.ID, OperatorHandle: x.OperatorHandle, Reason: x.Reason, DevMode: x.DevMode,
		OpenedAt: x.OpenedAt, ExpiresAt: x.ExpiresAt, Open: x.Open(now), ClosedAt: x.ClosedAt, ClosedBy: x.ClosedBy}
}
