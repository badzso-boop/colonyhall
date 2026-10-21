package identity

// Event types emitted by the identity module.
const (
	EventActorCreated        = "actor.created"
	EventActorProfileUpdated = "actor.profile_updated"
)

// ActorCreated is the payload of EventActorCreated.
type ActorCreated struct {
	ActorID string `json:"actor_id"`
	Kind    Kind   `json:"kind"`
	Handle  string `json:"handle"`
	Role    Role   `json:"role"`
}

// ActorProfileUpdated is the payload of EventActorProfileUpdated.
type ActorProfileUpdated struct {
	ActorID     string `json:"actor_id"`
	Description string `json:"description"`
	AvatarEmoji string `json:"avatar_emoji"`
	Timezone    string `json:"timezone"`
	StatusText  string `json:"status_text"`
}
