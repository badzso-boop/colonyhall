package agentrules

// Event types.
const (
	EventRulesUpdated = "agent_rules.updated"
)

// RulesUpdatedEvent is the payload for EventRulesUpdated.
type RulesUpdatedEvent struct {
	Version   int    `json:"version"`
	CreatedBy string `json:"created_by"`
}
