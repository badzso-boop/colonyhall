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
	// UpdateChannel stores a new name and description.
	UpdateChannel(ctx context.Context, id ids.ID, name, description string) error
	// SetArchived archives (at, by set) or unarchives (both nil) a channel.
	SetArchived(ctx context.Context, id ids.ID, at *time.Time, by *ids.ID) error
	// SetAgentLocked locks (at, by set) or unlocks (both nil) a channel.
	SetAgentLocked(ctx context.Context, id ids.ID, at *time.Time, by *ids.ID) error
	// AgentLockedChannels lists the workspace's channels locked against agents.
	AgentLockedChannels(ctx context.Context, workspaceID ids.ID) ([]ids.ID, error)

	// SetMute stores (or replaces) a viewer's mute; DeleteMute removes it.
	SetMute(ctx context.Context, m Mute) error
	DeleteMute(ctx context.Context, actorID, channelID ids.ID) error
	// Mutes returns the actor's mutes, including expired ones.
	Mutes(ctx context.Context, actorID ids.ID) ([]Mute, error)

	// SetThreadRead records that the actor has seen the thread up to at.
	SetThreadRead(ctx context.Context, actorID, rootID ids.ID, at time.Time) error
	// ThreadsForActor lists the threads (roots with replies, not retracted)
	// the actor wrote, replied in or was mentioned in, in channels the actor
	// is a member of, newest reply first, after the cursor (nil = start).
	// UnreadReplies counts others' replies after the later of the actor's
	// last read and the actor's own latest reply. unreadOnly keeps those
	// with unread replies.
	ThreadsForActor(ctx context.Context, workspaceID, actorID ids.ID, after *ThreadCursor, limit int, unreadOnly bool) ([]ThreadSummary, error)

	AddMember(ctx context.Context, m Member) error
	RemoveMember(ctx context.Context, channelID, actorID ids.ID, at time.Time) error
	Member(ctx context.Context, channelID, actorID ids.ID) (Member, error)
	// SetMemberCanWrite changes an active member's write permission;
	// db.ErrNotFound when the actor is not a member.
	SetMemberCanWrite(ctx context.Context, channelID, actorID ids.ID, canWrite bool) error
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	ListMembers(ctx context.Context, channelID ids.ID) ([]Member, error)

	InsertMessage(ctx context.Context, m Message) error
	MessageByID(ctx context.Context, id ids.ID) (Message, error)
	MessageByClientID(ctx context.Context, authorID ids.ID, clientMsgID string) (Message, error)
	MessagesByIDs(ctx context.Context, idList []ids.ID) ([]Message, error)
	// ListMessages returns up to limit messages older than before (or the
	// newest when before is ids.Nil), oldest first.
	ListMessages(ctx context.Context, channelID, before ids.ID, limit int) ([]Message, error)
	// ReplyParticipants returns, per thread root, up to max distinct authors
	// of its replies that are not retracted, the most recent replier first.
	ReplyParticipants(ctx context.Context, rootIDs []ids.ID, max int) (map[ids.ID][]ids.ID, error)

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
	// LatestTopLevelMessageByAuthor returns the newest top-level message in the channel by the author.
	LatestTopLevelMessageByAuthor(ctx context.Context, channelID, authorID ids.ID) (Message, error)
	// InsertPin pins a message; db.ErrConflict when it is pinned already.
	InsertPin(ctx context.Context, p Pin) error
	// Unpin marks the message's active pin; it reports whether there was one.
	Unpin(ctx context.Context, messageID, by ids.ID, at time.Time) (bool, error)
	// ActivePins lists the conversation's pins, newest first.
	ActivePins(ctx context.Context, channelID ids.ID) ([]Pin, error)
	CountActivePins(ctx context.Context, channelID ids.ID) (int, error)
	// CountPinsBy counts the actor's pins in the conversation since a time,
	// unpinned ones included.
	CountPinsBy(ctx context.Context, channelID, actorID ids.ID, since time.Time) (int, error)
	// PinnedAmong returns which of the messages are pinned now.
	PinnedAmong(ctx context.Context, messageIDs []ids.ID) (map[ids.ID]bool, error)

	// RetractMessage marks a message as retracted (soft delete).
	RetractMessage(ctx context.Context, id ids.ID, deletedBy ids.ID, at time.Time) error
}
