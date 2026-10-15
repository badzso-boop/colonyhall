package conversation

import "time"

// Event types emitted by the conversation module.
const (
	EventChannelCreated = "channel.created"
	EventMemberAdded    = "channel.member_added"
	EventMessageCreated = "message.created"
	EventReactionAdded  = "reaction.added"
	EventReactionRemove = "reaction.removed"
)

// ReactionEvent is the payload of the reaction events.
type ReactionEvent struct {
	MessageID       string `json:"message_id"`
	ChannelID       string `json:"channel_id"`
	ActorID         string `json:"actor_id"`
	MessageAuthorID string `json:"message_author_id"`
	Emoji           string `json:"emoji"`
}

// ChannelCreated is the payload of EventChannelCreated.
type ChannelCreated struct {
	ChannelID string      `json:"channel_id"`
	Kind      ChannelKind `json:"kind"`
	Name      string      `json:"name"`
}

// MemberAdded is the payload of EventMemberAdded.
type MemberAdded struct {
	ChannelID string `json:"channel_id"`
	ActorID   string `json:"actor_id"`
	CanWrite  bool   `json:"can_write"`
}

// MessageCreated is the payload of EventMessageCreated. It carries the whole
// message so the gateway can forward it without another query.
type MessageCreated struct {
	MessageID    string    `json:"message_id"`
	ChannelID    string    `json:"channel_id"`
	ThreadRootID string    `json:"thread_root_id,omitempty"`
	AuthorID     string    `json:"author_id"`
	ClientMsgID  string    `json:"client_msg_id,omitempty"`
	Kind         string    `json:"kind"`
	Body         string    `json:"body"`
	Mentions     []string  `json:"mentions"`
	CreatedAt    time.Time `json:"created_at"`
}
