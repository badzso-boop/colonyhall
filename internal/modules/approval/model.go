// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package approval

import (
	"errors"
	"time"

	"colonyhall/internal/modules/runs"
	"colonyhall/internal/platform/ids"
)

// Limits.
const (
	MaxInput       = 8000
	DefaultTimeout = 10 * time.Minute
)

// Approval states.
const (
	StatePending = "pending"
	StateAllowed = "allowed"
	StateDenied  = "denied"
	StateExpired = "expired"
)

// Decisions.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
)

// ApprovalRequest is one tool approval request from an agent.
type ApprovalRequest struct {
	ID              ids.ID
	WorkspaceID     ids.ID
	AgentID         ids.ID
	ChannelID       ids.ID
	ThreadRootID    ids.ID
	ToolName        string
	Input           string
	State           string
	DecidedBy       ids.ID
	DecisionMessage string
	CreatedAt       time.Time
	DecidedAt       *time.Time
	ExpiresAt       time.Time
}

// Errors.
var (
	ErrForbidden      = errors.New("not allowed")
	ErrAlreadyDecided = errors.New("approval request is not pending")
	ErrBadDecision    = errors.New("decision must be allow or deny")
	ErrExpired        = errors.New("approval request has expired")
	ErrBadToolName    = errors.New("tool name is required")
	// ErrInputTooLong: the input does not fit the approval card. Approving
	// a clipped input would run a part nobody saw (round 40), so such a
	// call is denied without asking.
	ErrInputTooLong = errors.New("the tool input is too long to show for approval; split it into smaller steps")
)

// Redact masks secrets in s using the same patterns as the runs module.
func Redact(s string) string {
	return runs.Redact(s)
}
