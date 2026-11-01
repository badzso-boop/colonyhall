package agents

// Event types of the agents module.
const (
	EventAgentCreated  = "agent.created"
	EventAgentArchived = "agent.archived"
)

// AgentEvent is the payload of agent events (no configuration details: those
// are in the host and actor events of the same transaction).
type AgentEvent struct {
	AgentID string `json:"agent_id"`
	Handle  string `json:"handle"`
	HostID  string `json:"host_id,omitempty"`
}
