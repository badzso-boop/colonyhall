package conversation

import (
	"context"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
)

// Thread returns a thread root and its replies (oldest first). Asking with
// a reply's ID returns that reply's whole thread.
func (s *Service) Thread(ctx context.Context, by identity.Actor, messageID ids.ID) (Message, []Message, error) {
	m, err := s.Store.MessageByID(ctx, messageID)
	if err != nil {
		return Message{}, nil, err
	}
	ok, err := s.CanRead(ctx, by, m.ChannelID)
	if err != nil {
		return Message{}, nil, err
	}
	if !ok {
		return Message{}, nil, ErrForbidden
	}
	rootID := m.ID
	if m.ThreadRootID != ids.Nil {
		rootID = m.ThreadRootID
	}
	all, err := s.Store.ThreadMessages(ctx, rootID)
	if err != nil {
		return Message{}, nil, err
	}
	all, err = s.withReactions(ctx, all)
	if err != nil {
		return Message{}, nil, err
	}
	all, err = s.withAttachments(ctx, all)
	if err != nil {
		return Message{}, nil, err
	}
	s.enrichOrigins(ctx, by, all)
	all = maskRetracted(all)
	var root Message
	var replies []Message
	for _, x := range all {
		if x.ID == rootID {
			root = x
		} else {
			replies = append(replies, x)
		}
	}
	return root, replies, nil
}
