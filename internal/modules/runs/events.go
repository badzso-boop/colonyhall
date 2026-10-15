package runs

// EventErrorReported is recorded (without the detail) for the audit log.
const EventErrorReported = "run.error_reported"

// ErrorReported is the payload of EventErrorReported.
type ErrorReported struct {
	ErrorID string `json:"error_id"`
	AgentID string `json:"agent_id"`
	Summary string `json:"summary"`
}
