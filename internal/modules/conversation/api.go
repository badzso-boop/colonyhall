package conversation

import (
	"context"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// Reader is the narrow view other modules may use.
type Reader interface {
	MessageByID(ctx context.Context, id ids.ID) (Message, error)
	MessagesByIDs(ctx context.Context, idList []ids.ID) ([]Message, error)
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	// ThreadAuthorIDs lists who wrote the thread's root or a reply, each once.
	ThreadAuthorIDs(ctx context.Context, rootID ids.ID) ([]ids.ID, error)
	ChannelByID(ctx context.Context, id ids.ID) (Channel, error)
	// CanRead reports whether the actor may read the channel; channels in
	// another workspace are never readable.
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	// CanWrite reports whether the actor may post in the channel.
	CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	// MessagesInUnit returns messages in a unit (thread or main stream), oldest first.
	MessagesInUnit(ctx context.Context, channelID, threadRootID ids.ID) ([]Message, error)
	// AgentLockedChannels lists the workspace's channels locked against agents.
	AgentLockedChannels(ctx context.Context, workspaceID ids.ID) ([]ids.ID, error)
	// LatestTopLevelMessageByAuthor returns the newest top-level message in the channel by the author.
	LatestTopLevelMessageByAuthor(ctx context.Context, channelID, authorID ids.ID) (Message, error)
	// PostSystemMessage posts a system message to a unit with optional file attachments.
	PostSystemMessage(ctx context.Context, workspaceID, channelID, threadRootID, authorID ids.ID, body string, fileIDs []ids.ID) (Message, error)
}

var _ Reader = (*Service)(nil)
