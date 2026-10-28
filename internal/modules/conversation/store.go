package conversation

import (
	"context"
	"time"

	"colonyhall/internal/platform/ids"
)

// Store persists channels, members and messages. Implementations: postgres,
// memory. Lookups return db.ErrNotFound; unique violations return db.ErrConflict.
type Store interface {
	CreateChannel(ctx context.Context, c Channel) error
	ChannelByID(ctx context.Context, id ids.ID) (Channel, error)
	ChannelByName(ctx context.Context, workspaceID ids.ID, name string) (Channel, error)
	// ChannelsForActor lists channels the actor belongs to; all=true lists
	// every channel in the workspace (for admins), marking membership.
	ChannelsForActor(ctx context.Context, workspaceID, actorID ids.ID, all bool) ([]ChannelView, error)

	AddMember(ctx context.Context, m Member) error
	RemoveMember(ctx context.Context, channelID, actorID ids.ID, at time.Time) error
	Member(ctx context.Context, channelID, actorID ids.ID) (Member, error)
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	ListMembers(ctx context.Context, channelID ids.ID) ([]Member, error)

	InsertMessage(ctx context.Context, m Message) error
	MessageByID(ctx context.Context, id ids.ID) (Message, error)
	MessageByClientID(ctx context.Context, authorID ids.ID, clientMsgID string) (Message, error)
	MessagesByIDs(ctx context.Context, idList []ids.ID) ([]Message, error)
	// ListMessages returns up to limit messages older than before (or the
	// newest when before is ids.Nil), oldest first.
	ListMessages(ctx context.Context, channelID, before ids.ID, limit int) ([]Message, error)

	// IncrementMsgCount adds one to the channel's message count and returns it.
	IncrementMsgCount(ctx context.Context, channelID ids.ID) (int64, error)
	// SetSeen marks the member as having seen count messages and clears mentions.
	SetSeen(ctx context.Context, channelID, actorID ids.ID, count int64) error
	IncrementMentions(ctx context.Context, channelID ids.ID, actorIDs []ids.ID) error
	BumpThread(ctx context.Context, rootID ids.ID, at time.Time) error

	// AddReaction adds (or restores) a reaction; it reports whether anything changed.
	AddReaction(ctx context.Context, r Reaction) (bool, error)
	// RemoveReaction soft-removes a reaction; it reports whether anything changed.
	RemoveReaction(ctx context.Context, messageID, actorID ids.ID, emoji string, at time.Time) (bool, error)
	// ReactionsFor returns active reactions on the messages, oldest first.
	ReactionsFor(ctx context.Context, messageIDs []ids.ID) ([]Reaction, error)
	// ThreadMessages returns the root and its replies, oldest first.
	ThreadMessages(ctx context.Context, rootID ids.ID) ([]Message, error)
	// MainStreamMessages returns top-level messages in the channel, oldest first.
	MainStreamMessages(ctx context.Context, channelID ids.ID) ([]Message, error)
	// LastMessageNotBy returns the newest message in the channel by someone else.
	LastMessageNotBy(ctx context.Context, channelID, actorID ids.ID) (Message, error)
	// RetractMessage marks a message as retracted (soft delete).
	RetractMessage(ctx context.Context, id ids.ID, deletedBy ids.ID, at time.Time) error
}
