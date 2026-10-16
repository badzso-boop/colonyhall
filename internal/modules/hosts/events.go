package hosts

// Event types (all recorded in the audit log; admins see them live).
const (
	EventPairingCreated  = "host.pairing_created"
	EventHostEnrolled    = "host.enrolled"
	EventHostState       = "host.state_changed"
	EventAgentAssigned   = "host.agent_assigned"
	EventAgentUnassigned = "host.agent_unassigned"
	EventGrantAdded      = "host.grant_added"
	EventGrantRevoked    = "host.grant_revoked"
)

// HostEvent is the payload of host events.
type HostEvent struct {
	HostID string `json:"host_id,omitempty"`
	Name   string `json:"name,omitempty"`
	State  string `json:"state,omitempty"`
}

// AssignmentEvent is the payload of agent assignment events.
type AssignmentEvent struct {
	AgentID          string `json:"agent_id"`
	HostID           string `json:"host_id,omitempty"`
	Client           string `json:"client,omitempty"`
	Model            string `json:"model,omitempty"`
	AllowWork        bool   `json:"allow_work"`
	Mode             string `json:"mode,omitempty"`
	RootDir          string `json:"root_dir,omitempty"`
	Sandbox          bool   `json:"sandbox"`
	Approvals        string `json:"approvals,omitempty"`
	MaxSessionTokens int    `json:"max_session_tokens,omitempty"`
	DailyTokenLimit  int64  `json:"daily_token_limit,omitempty"`
}

// GrantEvent is the payload of grant events.
type GrantEvent struct {
	GrantID string `json:"grant_id"`
	AgentID string `json:"agent_id"`
	HostID  string `json:"host_id"`
	Path    string `json:"path"`
	Mode    string `json:"mode"`
}
