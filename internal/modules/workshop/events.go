// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package workshop

// Events (audited). Never a secret or a tool's input or output.
const (
	EventConnectorCreated   = "workshop.connector_created"
	EventConnectorUpdated   = "workshop.connector_updated"
	EventConnectorDeleted   = "workshop.connector_deleted"
	EventApprovalOffChanged = "workshop.approval_off_changed"
	EventRiskOverridden     = "workshop.risk_overridden"
	EventGrantAdded         = "workshop.grant_added"
	EventGrantRevoked       = "workshop.grant_revoked"

	EventExtensionCreated  = "workshop.extension_created"
	EventVersionAdded      = "workshop.version_added"
	EventVersionApproved   = "workshop.version_approved"
	EventRolledBack        = "workshop.rolled_back"
	EventExtensionEnabled  = "workshop.extension_enabled"
	EventExtensionDisabled = "workshop.extension_disabled"
	EventTakenOver         = "workshop.taken_over"
	EventExtensionDeleted  = "workshop.extension_deleted"
	EventWebhookCreated    = "workshop.webhook_created"
)

// ExtensionChanged is the payload of the extension events.
type ExtensionChanged struct {
	ExtensionID string `json:"extension_id"`
	Slug        string `json:"slug"`
	BotID       string `json:"bot_id,omitempty"`
	Version     int    `json:"version,omitempty"`
	// Diff is what an approved version added.
	Diff   *Diff  `json:"diff,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ConnectorChanged is the payload of the connector events.
type ConnectorChanged struct {
	ConnectorID string `json:"connector_id"`
	Slug        string `json:"slug"`
	Kind        string `json:"kind,omitempty"`
	// ApprovalOff is set on approval_off_changed.
	ApprovalOff *bool `json:"approval_off,omitempty"`
	// Tool and Risk are set on risk_overridden ("" risk: override removed).
	Tool string `json:"tool,omitempty"`
	Risk string `json:"risk,omitempty"`
}

// GrantChanged is the payload of the grant events.
type GrantChanged struct {
	GrantID    string `json:"grant_id"`
	AgentID    string `json:"agent_id"`
	TargetKind string `json:"target_kind"`
	TargetID   string `json:"target_id,omitempty"`
}
