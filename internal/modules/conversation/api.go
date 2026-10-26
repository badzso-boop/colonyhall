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
}

var _ Reader = (*Service)(nil)
