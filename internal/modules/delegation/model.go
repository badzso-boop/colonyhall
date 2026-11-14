// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package delegation

import (
	"time"

	"colonyhall/internal/platform/ids"
)

// ConsumerName is the event-log consumer name.
const ConsumerName = "delegation"

// Delegation states.
const (
	StatePending   = "pending"
	StateApproved  = "approved"
	StateDenied    = "denied"
	StateExpired   = "expired"
	StateCancelled = "cancelled"
)

// Event types.
const (
	EventCreated         = "delegation.created"
	EventApproved        = "delegation.approved"
	EventDenied          = "delegation.denied"
	EventExpired         = "delegation.expired"
	EventGrantCreated    = "delegation.grant_created"
	EventGrantRevoked    = "delegation.grant_revoked"
	EventSettingsUpdated = "delegation.settings_updated"
)

// ConversationKey identifies a conversation scope.
type ConversationKey struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID // ids.Nil for the whole DM
}

// CanonicalPair returns a and b in deterministic order (a < b).
func CanonicalPair(id1, id2 ids.ID) (ids.ID, ids.ID) {
	if id1.String() < id2.String() {
		return id1, id2
	}
	return id2, id1
}

// Delegation is a request for a human admin to approve an agent task for a full-access agent.
type Delegation struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	MessageID    ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID
	FromAgentID  ids.ID
	ToAgentID    ids.ID
	QuestionID   ids.ID
	State        string
	DecidedBy    *ids.ID
	DecidedAt    *time.Time
	ViaGrant     bool // approved through a conversation grant, not by an admin's answer
	CreatedAt    time.Time
}

// DelegationView joins a Delegation with actor handles for API responses.
type DelegationView struct {
	Delegation
	FromAgentHandle  string
	ToAgentHandle    string
	ApproverHandle   string
	DecidedByHandle  *string
	ApprovedByGrant  bool
	GrantIdleMinutes *int
	GrantMaxHours    *int
}

// Grant represents an active delegation grant between two agents in a conversation.
type Grant struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID // ids.Nil if covering the whole DM
	AgentAID     ids.ID // canonical order (AgentAID < AgentBID)
	AgentBID     ids.ID
	DelegationID ids.ID
	ApprovedBy   ids.ID
	ApprovedAt   time.Time
	LastUsedAt   time.Time
	ExpiresAt    time.Time
	RevokedBy    *ids.ID
	RevokedAt    *time.Time
}

// IsActive reports whether the grant is currently active given idle minutes limit.
func (g Grant) IsActive(now time.Time, idleMinutes int) bool {
	if g.RevokedAt != nil {
		return false
	}
	if !now.Before(g.ExpiresAt) {
		return false
	}
	idleDeadline := g.LastUsedAt.Add(time.Duration(idleMinutes) * time.Minute)
	return now.Before(idleDeadline)
}

// EffectiveExpiresAt returns the effective end time min(ExpiresAt, LastUsedAt + idleMinutes).
func (g Grant) EffectiveExpiresAt(idleMinutes int) time.Time {
	idleDeadline := g.LastUsedAt.Add(time.Duration(idleMinutes) * time.Minute)
	if idleDeadline.Before(g.ExpiresAt) {
		return idleDeadline
	}
	return g.ExpiresAt
}

// GrantView joins a Grant with actor handles for API responses.
type GrantView struct {
	Grant
	AgentAHandle    string
	AgentBHandle    string
	ApproverHandle  string
	RevokedByHandle *string
	IdleMinutes     int
}

// Settings holds per-workspace delegation settings.
type Settings struct {
	WorkspaceID      ids.ID
	GrantIdleMinutes int
	GrantMaxHours    int
	UpdatedAt        time.Time
}

const (
	DefaultGrantIdleMinutes = 60
	MinGrantIdleMinutes     = 5
	MaxGrantIdleMinutes     = 1440

	DefaultGrantMaxHours = 12
	MinGrantMaxHours     = 1
	MaxGrantMaxHours     = 72
)

// DefaultSettings returns default settings for a workspace.
func DefaultSettings(workspaceID ids.ID) Settings {
	return Settings{
		WorkspaceID:      workspaceID,
		GrantIdleMinutes: DefaultGrantIdleMinutes,
		GrantMaxHours:    DefaultGrantMaxHours,
	}
}

// EventPayload is the payload of a delegation event.
type EventPayload struct {
	DelegationID string  `json:"delegation_id"`
	MessageID    string  `json:"message_id"`
	ChannelID    string  `json:"channel_id"`
	ThreadRootID string  `json:"thread_root_id,omitempty"`
	FromAgentID  string  `json:"from_agent_id"`
	ToAgentID    string  `json:"to_agent_id"`
	QuestionID   string  `json:"question_id"`
	State        string  `json:"state"`
	DecidedBy    *string `json:"decided_by,omitempty"`
}

// GrantEventPayload is the payload of a delegation grant event.
type GrantEventPayload struct {
	GrantID            string  `json:"grant_id"`
	WorkspaceID        string  `json:"workspace_id"`
	ChannelID          string  `json:"channel_id"`
	ThreadRootID       string  `json:"thread_root_id,omitempty"`
	AgentAID           string  `json:"agent_a_id"`
	AgentBID           string  `json:"agent_b_id"`
	DelegationID       string  `json:"delegation_id"`
	ApprovedBy         string  `json:"approved_by"`
	ApprovedAt         string  `json:"approved_at"`
	ExpiresAt          string  `json:"expires_at"`
	EffectiveExpiresAt string  `json:"effective_expires_at,omitempty"`
	RevokedBy          *string `json:"revoked_by,omitempty"`
	RevokedAt          *string `json:"revoked_at,omitempty"`
}

// SettingsEventPayload is the payload of a settings updated event.
type SettingsEventPayload struct {
	WorkspaceID      string `json:"workspace_id"`
	GrantIdleMinutes int    `json:"grant_idle_minutes"`
	GrantMaxHours    int    `json:"grant_max_hours"`
}
