// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/ids"
)

// BreakGlass is an operator's read-only access to a workspace (round 51b,
// docs/24 6. pont), as the operator sees it.
type BreakGlass struct {
	ID             ids.ID
	OperatorHandle string
	Reason         string
	DevMode        bool
	OpenedAt       time.Time
	ExpiresAt      time.Time
	Open           bool
	ClosedAt       *time.Time
	ClosedBy       string
}

// BreakGlassLink is a new access and its handoff token (shown once).
type BreakGlassLink struct {
	BreakGlass       BreakGlass
	Token            string
	HandoffExpiresAt time.Time
}

// Break-glass limits.
const (
	BreakGlassMinReason  = 20
	BreakGlassMaxReason  = 500
	BreakGlassMaxMinutes = 240
	// BreakGlassDevMaxMinutes: development mode allows longer.
	BreakGlassDevMaxMinutes = 720
)

// ErrInvalidBreakGlass: a reason too short or long, or a time out of range.
var ErrInvalidBreakGlass = errors.New("break-glass access needs a reason of 20–500 characters and 1–240 minutes")

// BreakGlassAccess is what the operator module needs from the identity
// module (the composition root wires it; each call narrows to the one
// workspace it concerns).
type BreakGlassAccess interface {
	OpenBreakGlass(ctx context.Context, workspaceID ids.ID, o Operator, reason string, expiresAt time.Time, devMode bool) (BreakGlassLink, error)
	ListBreakGlass(ctx context.Context, workspaceID ids.ID) ([]BreakGlass, error)
	CloseBreakGlass(ctx context.Context, workspaceID, id ids.ID) error
}

// OpenBreakGlass opens a read-only access to a workspace for minutes, with
// a reason (development mode: optional, and up to 12 hours). The operator
// audit records it; the workspace's audit and owners hear of it from the
// identity module.
func (s *Service) OpenBreakGlass(ctx context.Context, o Operator, workspaceID ids.ID, reason string, minutes int, remote string) (BreakGlassLink, error) {
	reason = strings.TrimSpace(reason)
	n := utf8.RuneCountInString(reason)
	maxMin := BreakGlassMaxMinutes
	if s.DevAccess {
		maxMin = BreakGlassDevMaxMinutes
	}
	if minutes < 1 || minutes > maxMin || n > BreakGlassMaxReason || (!s.DevAccess && n < BreakGlassMinReason) {
		return BreakGlassLink{}, ErrInvalidBreakGlass
	}
	ctx = scope(ctx)
	if _, err := s.Customers.Get(ctx, workspaceID); err != nil {
		return BreakGlassLink{}, err
	}
	link, err := s.BreakGlass.OpenBreakGlass(ctx, workspaceID, o, reason, s.Clock.Now().Add(time.Duration(minutes)*time.Minute), s.DevAccess)
	if err != nil {
		return BreakGlassLink{}, err
	}
	return link, s.record(ctx, o, ActionBreakGlassOpen, workspaceID, map[string]any{"break_glass_id": link.BreakGlass.ID.String(),
		"reason": reason, "minutes": minutes, "dev_mode": s.DevAccess}, remote)
}

// ListBreakGlass lists a workspace's accesses, newest first.
func (s *Service) ListBreakGlass(ctx context.Context, workspaceID ids.ID) ([]BreakGlass, error) {
	ctx = scope(ctx)
	if _, err := s.Customers.Get(ctx, workspaceID); err != nil {
		return nil, err
	}
	return s.BreakGlass.ListBreakGlass(ctx, workspaceID)
}

// CloseBreakGlass closes an access at once.
func (s *Service) CloseBreakGlass(ctx context.Context, o Operator, workspaceID, id ids.ID, remote string) error {
	ctx = scope(ctx)
	if err := s.BreakGlass.CloseBreakGlass(ctx, workspaceID, id); err != nil {
		return err
	}
	return s.record(ctx, o, ActionBreakGlassClose, workspaceID, map[string]any{"break_glass_id": id.String()}, remote)
}
