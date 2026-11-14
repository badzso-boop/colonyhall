// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package hosts

// Event types (all recorded in the audit log; admins see them live).
const (
	EventPairingCreated = "host.pairing_created"
	EventHostEnrolled   = "host.enrolled"
	EventHostState      = "host.state_changed"
	// EventHostCrossTenant: the host token was used on another workspace's
	// subdomain; the host is revoked (round 45).
	EventHostCrossTenant = "host.cross_tenant"
	EventHostUpdated     = "host.updated"
	EventHostOwner       = "host.owner_changed"
	EventAgentAssigned   = "host.agent_assigned"
	EventAgentUnassigned = "host.agent_unassigned"
	EventAgentLimits     = "host.agent_limits_changed"
	EventGrantAdded      = "host.grant_added"
	EventGrantRevoked    = "host.grant_revoked"
)

// HostEvent is the payload of host events.
type HostEvent struct {
	HostID string `json:"host_id,omitempty"`
	Name   string `json:"name,omitempty"`
	State  string `json:"state,omitempty"`
	Color  string `json:"color,omitempty"`
	// Owner is the new owner's id on host.owner_changed ("" is nobody).
	Owner string `json:"owner,omitempty"`
}

// AssignmentEvent is the payload of agent assignment events.
type AssignmentEvent struct {
	AgentID          string `json:"agent_id"`
	HostID           string `json:"host_id,omitempty"`
	Client           string `json:"client,omitempty"`
	Model            string `json:"model,omitempty"`
	AllowWork        bool   `json:"allow_work"`
	Mode             string `json:"mode,omitempty"`
	RootDir          string `json:"root_dir,omitempty"`
	Sandbox          bool   `json:"sandbox"`
	Approvals        string `json:"approvals,omitempty"`
	MaxSessionTokens int    `json:"max_session_tokens,omitempty"`
	DailyTokenLimit  int64  `json:"daily_token_limit,omitempty"`
	WorktreeRepo     string `json:"worktree_repo,omitempty"`
	AllowPreview     bool   `json:"allow_preview"`
	AllowWebRead     bool   `json:"allow_web_read"`
	WriteScope       string `json:"write_scope,omitempty"`
	PartnerID        string `json:"partner_id,omitempty"`
}

// GrantEvent is the payload of grant events.
type GrantEvent struct {
	GrantID string `json:"grant_id"`
	AgentID string `json:"agent_id"`
	HostID  string `json:"host_id"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
}
