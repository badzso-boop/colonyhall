// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package agentrules

// Event types.
const (
	EventRulesUpdated = "agent_rules.updated"
)

// RulesUpdatedEvent is the payload for EventRulesUpdated.
type RulesUpdatedEvent struct {
	Version   int    `json:"version"`
	CreatedBy string `json:"created_by"`
}
