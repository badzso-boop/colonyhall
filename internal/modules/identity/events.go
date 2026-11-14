// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

// Event types emitted by the identity module.
const (
	EventActorCreated        = "actor.created"
	EventActorProfileUpdated = "actor.profile_updated"
	EventActorArchived       = "actor.archived"
	// EventCrossTenant: a credential was used on another workspace's
	// subdomain and has been revoked (round 45). Recorded in the
	// credential's own workspace, so its admins see it in the audit log.
	EventCrossTenant = "auth.cross_tenant"
)

// CrossTenantUse is the payload of EventCrossTenant. No secret: only the
// credential's kind and visible prefix and where it was used.
type CrossTenantUse struct {
	Credential string `json:"credential"` // "token", "session" or "host"
	Prefix     string `json:"prefix,omitempty"`
	ActorID    string `json:"actor_id,omitempty"`
	UsedOn     string `json:"used_on"` // the other workspace's subdomain
	RemoteAddr string `json:"remote_addr,omitempty"`
}

// ActorArchived is the payload of EventActorArchived.
type ActorArchived struct {
	ActorID string `json:"actor_id"`
	Handle  string `json:"handle"`
}

// ActorCreated is the payload of EventActorCreated.
type ActorCreated struct {
	ActorID string `json:"actor_id"`
	Kind    Kind   `json:"kind"`
	Handle  string `json:"handle"`
	Role    Role   `json:"role"`
}

// ActorProfileUpdated is the payload of EventActorProfileUpdated.
type ActorProfileUpdated struct {
	ActorID     string `json:"actor_id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	AvatarEmoji string `json:"avatar_emoji"`
	Timezone    string `json:"timezone"`
	StatusText  string `json:"status_text"`
	Color       string `json:"color,omitempty"`
}
