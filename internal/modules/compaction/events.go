// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package compaction

const (
	ConsumerName = "compaction"

	EventCreated = "compaction.created"
	EventDone    = "compaction.done"
	EventFailed  = "compaction.failed"
)

type CompactionCreated struct {
	CompactionID     string `json:"compaction_id"`
	WorkspaceID      string `json:"workspace_id"`
	ChannelID        string `json:"channel_id"`
	ThreadRootID     string `json:"thread_root_id,omitempty"`
	CompactorAgentID string `json:"compactor_agent_id"`
	RequestedBy      string `json:"requested_by,omitempty"`
	Trigger          string `json:"trigger"`
}

type CompactionDone struct {
	CompactionID    string `json:"compaction_id"`
	WorkspaceID     string `json:"workspace_id"`
	ChannelID       string `json:"channel_id"`
	ThreadRootID    string `json:"thread_root_id,omitempty"`
	SummaryFileID   string `json:"summary_file_id"`
	SystemMessageID string `json:"system_message_id"`
}

type CompactionFailed struct {
	CompactionID string `json:"compaction_id"`
	WorkspaceID  string `json:"workspace_id"`
	ChannelID    string `json:"channel_id"`
	ErrorCode    string `json:"error_code"`
}

// EventSettingsUpdated: an admin changed the threshold or the scribe.
const EventSettingsUpdated = "compaction.settings_updated"

// SettingsUpdated is the payload of EventSettingsUpdated.
type SettingsUpdated struct {
	WorkspaceID            string `json:"workspace_id"`
	UpdatedBy              string `json:"updated_by"`
	CompactThresholdTokens int    `json:"compact_threshold_tokens"`
	CompactorAgentID       string `json:"compactor_agent_id"`
}
