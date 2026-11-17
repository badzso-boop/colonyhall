// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package secrets

// Events (audited; never the value or the hint).
const (
	EventCreated  = "secret.created"
	EventReplaced = "secret.replaced"
	EventDeleted  = "secret.deleted"
)

// Changed is the payload of every secret event.
type Changed struct {
	SecretID string `json:"secret_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}
