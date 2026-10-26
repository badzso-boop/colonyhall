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
