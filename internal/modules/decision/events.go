// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package decision

// Event types emitted by the decision module.
const (
	// EventRecorded: a decision was recorded (round 36: the search indexes
	// decisions from it, as their own kind).
	EventRecorded = "decision.recorded"
)

// Recorded is the payload of EventRecorded.
type Recorded struct {
	DecisionID   string `json:"decision_id"`
	Number       int    `json:"number"`
	ChannelID    string `json:"channel_id"`
	ThreadRootID string `json:"thread_root_id,omitempty"`
	AuthorID     string `json:"author_id"`
	Text         string `json:"text"`
	Rationale    string `json:"rationale"`
	Supersedes   *int   `json:"supersedes,omitempty"`
	WikiSlug     string `json:"wiki_slug"`
}
