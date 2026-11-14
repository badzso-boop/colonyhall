package conversation

import "time"

// Event types emitted by the conversation module.
const (
	EventChannelCreated = "channel.created"
	EventMemberAdded    = "channel.member_added"
	EventMemberRemoved  = "channel.member_removed"
	// EventMemberUpdated: a member's write permission changed (payload MemberAdded).
	EventMemberUpdated    = "channel.member_updated"
	EventChannelUpdated   = "channel.updated"
	EventChannelArchived  = "channel.archived"
	EventChannelUnarchive = "channel.unarchived"
	EventChannelLocked    = "channel.locked"
	EventChannelUnlocked  = "channel.unlocked"
	// EventMuteChanged reaches only the viewer who muted (their other tabs).
	EventMuteChanged      = "channel.mute_changed"
	EventMessageCreated   = "message.created"
	EventMessageRetracted = "message.retracted"
	EventReactionAdded    = "reaction.added"
	EventReactionRemove   = "reaction.removed"
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

// ChannelUpdated is the payload of EventChannelUpdated. It never carries the
// description text: clients refetch the channel.
type ChannelUpdated struct {
	ChannelID string   `json:"channel_id"`
	Fields    []string `json:"fields"`
	OldName   string   `json:"old_name,omitempty"`
	NewName   string   `json:"new_name,omitempty"`
}

// ChannelArchived is the payload of EventChannelArchived and EventChannelUnarchive.
type ChannelArchived struct {
	ChannelID string `json:"channel_id"`
	Name      string `json:"name"`
}

// ChannelLocked is the payload of EventChannelLocked and EventChannelUnlocked.
type ChannelLocked struct {
	ChannelID string `json:"channel_id"`
	Name      string `json:"name"`
}

// MuteChanged is the payload of EventMuteChanged.
type MuteChanged struct {
	ChannelID  string     `json:"channel_id"`
	ActorID    string     `json:"actor_id"`
	Muted      bool       `json:"muted"`
	MutedUntil *time.Time `json:"muted_until,omitempty"`
}

// MemberRemoved is the payload of EventMemberRemoved.
type MemberRemoved struct {
	ChannelID string `json:"channel_id"`
	ActorID   string `json:"actor_id"`
}

// MessageCreated is the payload of EventMessageCreated. It carries the whole
// message so the gateway can forward it without another query.
type MessageCreated struct {
	MessageID    string   `json:"message_id"`
	ChannelID    string   `json:"channel_id"`
	ThreadRootID string   `json:"thread_root_id,omitempty"`
	AuthorID     string   `json:"author_id"`
	ClientMsgID  string   `json:"client_msg_id,omitempty"`
	Kind         string   `json:"kind"`
	Body         string   `json:"body"`
	Mentions     []string `json:"mentions"`
	// NoThreadFollow: delivered to the thread's agents another way.
	NoThreadFollow bool         `json:"no_thread_follow,omitempty"`
	Silent         bool         `json:"silent,omitempty"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	Important      bool         `json:"important,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
}

// MessageRetracted is the payload of EventMessageRetracted.
type MessageRetracted struct {
	MessageID    string    `json:"message_id"`
	ChannelID    string    `json:"channel_id"`
	ThreadRootID string    `json:"thread_root_id,omitempty"`
	AuthorID     string    `json:"author_id"`
	RetractedBy  string    `json:"retracted_by"`
	At           time.Time `json:"at"`
}
