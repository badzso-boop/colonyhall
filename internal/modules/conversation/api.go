package conversation

import (
	"context"

	"colonyhall/internal/platform/ids"
)

// Reader is the narrow view other modules may use.
type Reader interface {
	MessagesByIDs(ctx context.Context, idList []ids.ID) ([]Message, error)
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	ChannelByID(ctx context.Context, id ids.ID) (Channel, error)
	// CanRead reports whether the actor may read the channel.
	CanRead(ctx context.Context, actorID ids.ID, isAdmin bool, channelID ids.ID) (bool, error)
}

var _ Reader = (*Service)(nil)
