package identity

// Event types emitted by the identity module.
const (
	EventActorCreated = "actor.created"
)

// ActorCreated is the payload of EventActorCreated.
type ActorCreated struct {
	ActorID string `json:"actor_id"`
	Kind    Kind   `json:"kind"`
	Handle  string `json:"handle"`
	Role    Role   `json:"role"`
}
