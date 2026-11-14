// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package runs

// EventErrorReported is recorded (without the detail) for the audit log.
const EventErrorReported = "run.error_reported"

// ErrorReported is the payload of EventErrorReported.
type ErrorReported struct {
	ErrorID string `json:"error_id"`
	AgentID string `json:"agent_id"`
	Summary string `json:"summary"`
}

// EventRunReported is recorded for the audit log when an agent run finishes.
const EventRunReported = "run.reported"

// RunReported is the payload of EventRunReported.
type RunReported struct {
	RunID       string `json:"run_id"`
	AgentID     string `json:"agent_id"`
	Phase       string `json:"phase"`
	Client      string `json:"client"`
	Model       string `json:"model"`
	Items       int    `json:"items"`
	TotalTokens int64  `json:"total_tokens"`
	DurationMS  int    `json:"duration_ms"`
	Status      string `json:"status"`
}
