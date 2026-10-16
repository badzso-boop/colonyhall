package approval

// Events for the audit log. Input is never included in the payload.
const (
	EventApprovalRequested = "approval.requested"
	EventApprovalDecided   = "approval.decided"
)

// ApprovalRequested is the payload of EventApprovalRequested.
type ApprovalRequested struct {
	ApprovalID string `json:"approval_id"`
	AgentID    string `json:"agent_id"`
	ChannelID  string `json:"channel_id"`
	ToolName   string `json:"tool_name"`
}

// ApprovalDecided is the payload of EventApprovalDecided.
type ApprovalDecided struct {
	ApprovalID string `json:"approval_id"`
	DecidedBy  string `json:"decided_by"`
	Decision   string `json:"decision"`
	State      string `json:"state"`
}
