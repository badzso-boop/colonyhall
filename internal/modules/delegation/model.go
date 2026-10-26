package delegation

import (
	"time"

	"colonyhall/internal/platform/ids"
)

// ConsumerName is the event-log consumer name.
const ConsumerName = "delegation"

// Delegation states.
const (
	StatePending  = "pending"
	StateApproved = "approved"
	StateDenied   = "denied"
	StateExpired  = "expired"
)

// Event types.
const (
	EventCreated  = "delegation.created"
	EventApproved = "delegation.approved"
	EventDenied   = "delegation.denied"
	EventExpired  = "delegation.expired"
)

// Delegation is a request for a human admin to approve an agent task for a full-access agent.
type Delegation struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	MessageID    ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID
	FromAgentID  ids.ID
	ToAgentID    ids.ID
	QuestionID   ids.ID
	State        string
	DecidedBy    *ids.ID
	DecidedAt    *time.Time
	CreatedAt    time.Time
}

// DelegationView joins a Delegation with actor handles for API responses.
type DelegationView struct {
	Delegation
	FromAgentHandle string
	ToAgentHandle   string
	DecidedByHandle *string
}

// EventPayload is the payload of a delegation event.
type EventPayload struct {
	DelegationID string  `json:"delegation_id"`
	MessageID    string  `json:"message_id"`
	ChannelID    string  `json:"channel_id"`
	ThreadRootID string  `json:"thread_root_id,omitempty"`
	FromAgentID  string  `json:"from_agent_id"`
	ToAgentID    string  `json:"to_agent_id"`
	QuestionID   string  `json:"question_id"`
	State        string  `json:"state"`
	DecidedBy    *string `json:"decided_by,omitempty"`
}
