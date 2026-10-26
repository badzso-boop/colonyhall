package compaction

import (
	"time"

	"colonyhall/internal/platform/ids"
)

const (
	DefaultThresholdTokens = 350000
	MinThresholdTokens     = 20000
	MaxThresholdTokens     = 2000000
	DefaultMaxSummaryChars = 40000
)

type Trigger string

const (
	TriggerAuto       Trigger = "auto"
	TriggerManual     Trigger = "manual"
	TriggerRegenerate Trigger = "regenerate"
	TriggerEdit       Trigger = "edit"
)

type State string

const (
	StatePending State = "pending"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

type Settings struct {
	WorkspaceID            ids.ID    `json:"workspace_id"`
	CompactThresholdTokens int       `json:"compact_threshold_tokens"`
	CompactorAgentID       ids.ID    `json:"compactor_agent_id,omitempty"`
	CompactorHandle        string    `json:"compactor_handle,omitempty"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type CompactionUnit struct {
	ID                ids.ID    `json:"id"`
	WorkspaceID       ids.ID    `json:"workspace_id"`
	ChannelID         ids.ID    `json:"channel_id"`
	ThreadRootID      ids.ID    `json:"thread_root_id,omitempty"`
	ApproxTokensSince int       `json:"approx_tokens_since"`
	MessagesSince     int       `json:"messages_since"`
	LastCompactionID  ids.ID    `json:"last_compaction_id,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Stats struct {
	Messages       int `json:"messages"`
	ApproxTokensIn int `json:"approx_tokens_in"`
	SummaryTokens  int `json:"summary_tokens"`
}

type Compaction struct {
	ID                   ids.ID     `json:"id"`
	WorkspaceID          ids.ID     `json:"workspace_id"`
	ChannelID            ids.ID     `json:"channel_id"`
	ThreadRootID         ids.ID     `json:"thread_root_id,omitempty"`
	UpToMessageID        ids.ID     `json:"up_to_message_id,omitempty"`
	PreviousCompactionID ids.ID     `json:"previous_compaction_id,omitempty"`
	Trigger              Trigger    `json:"trigger"`
	RequestedBy          ids.ID     `json:"requested_by,omitempty"`
	CompactorAgentID     ids.ID     `json:"compactor_agent_id,omitempty"`
	State                State      `json:"state"`
	SummaryFileID        ids.ID     `json:"summary_file_id,omitempty"`
	SystemMessageID      ids.ID     `json:"system_message_id,omitempty"`
	MessagesCount        int        `json:"messages_count"`
	ApproxTokensIn       int        `json:"approx_tokens_in"`
	SummaryTokens        int        `json:"summary_tokens"`
	ErrorCode            string     `json:"error_code,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
}

type TaskMessage struct {
	ID           ids.ID    `json:"id"`
	AuthorHandle string    `json:"author_handle"`
	AuthorKind   string    `json:"author_kind"`
	Body         string    `json:"body"`
	Attachments  []string  `json:"attachments"`
	CreatedAt    time.Time `json:"created_at"`
}

type ScribeTask struct {
	ID              ids.ID        `json:"id"`
	ChannelID       ids.ID        `json:"channel_id"`
	ThreadRootID    ids.ID        `json:"thread_root_id,omitempty"`
	PreviousSummary string        `json:"previous_summary,omitempty"`
	Messages        []TaskMessage `json:"messages"`
	MaxSummaryChars int           `json:"max_summary_chars"`
}

type ContextMessage struct {
	ID           ids.ID    `json:"id"`
	AuthorHandle string    `json:"author_handle"`
	AuthorKind   string    `json:"author_kind"`
	Body         string    `json:"body"`
	Attachments  []string  `json:"attachments"`
	CreatedAt    time.Time `json:"created_at"`
}

type AgentContext struct {
	CompactionID    ids.ID           `json:"compaction_id,omitempty"`
	SummaryMarkdown string           `json:"summary_markdown,omitempty"`
	MessagesAfter   []ContextMessage `json:"messages_after"`
	HasMore         bool             `json:"has_more"`
}
