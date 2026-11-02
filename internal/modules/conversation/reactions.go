package conversation

import (
	"context"
	"unicode"
	"unicode/utf8"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/policy"
)

// MaxEmojiLen bounds a reaction string.
const MaxEmojiLen = 32

// ThumbsUp and ThumbsDown carry meaning for agents (agree / disagree).
const (
	ThumbsUp   = "👍"
	ThumbsDown = "👎"
)

// validEmoji accepts short emoji sequences: at least one non-ASCII rune, and
// no ASCII besides the keycap characters (0-9 # *). No markup, no text.
func validEmoji(e string) bool {
	if e == "" || utf8.RuneCountInString(e) > MaxEmojiLen {
		return false
	}
	wide := false
	for _, r := range e {
		switch {
		case r >= utf8.RuneSelf:
			if unicode.IsControl(r) || unicode.IsSpace(r) || unicode.IsLetter(r) {
				return false
			}
			wide = true
		case r >= '0' && r <= '9', r == '#', r == '*':
		default:
			return false
		}
	}
	return wide
}

// React adds a reaction. Reacting twice with the same emoji is a no-op.
func (s *Service) React(ctx context.Context, by identity.Actor, messageID ids.ID, emoji string) error {
	return s.setReaction(ctx, by, messageID, emoji, true)
}

// Unreact removes the caller's reaction.
func (s *Service) Unreact(ctx context.Context, by identity.Actor, messageID ids.ID, emoji string) error {
	return s.setReaction(ctx, by, messageID, emoji, false)
}

func (s *Service) setReaction(ctx context.Context, by identity.Actor, messageID ids.ID, emoji string, add bool) error {
	if !validEmoji(emoji) {
		return ErrBadEmoji
	}
	if by.Halted {
		return identity.ErrHalted
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		seq, err = s.reactInTx(ctx, by, messageID, emoji, add)
		return err
	})
	if err != nil {
		return err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return nil
}

// reactInTx changes a reaction inside the caller's transaction and returns
// the event seq (0 when nothing changed).
func (s *Service) reactInTx(ctx context.Context, by identity.Actor, messageID ids.ID, emoji string, add bool) (int64, error) {
	m, err := s.Store.MessageByID(ctx, messageID)
	if err != nil {
		return 0, err
	}
	c, err := s.channelFor(ctx, by.WorkspaceID, m.ChannelID)
	if err != nil {
		return 0, err
	}
	mem, err := s.membership(ctx, c.ID, by.ID)
	if err != nil {
		return 0, err
	}
	if !policy.CanWriteChannel(subject(by), mem) {
		return 0, ErrForbidden
	}
	if c.Archived() {
		return 0, ErrChannelArchived
	}
	var changed bool
	typ := EventReactionAdded
	if add {
		changed, err = s.Store.AddReaction(ctx, Reaction{MessageID: m.ID, ActorID: by.ID, Emoji: emoji, CreatedAt: s.Clock.Now()})
	} else {
		typ = EventReactionRemove
		changed, err = s.Store.RemoveReaction(ctx, m.ID, by.ID, emoji, s.Clock.Now())
	}
	if err != nil || !changed {
		return 0, err
	}
	return s.appendEvent(ctx, by, c.ID, typ, "message", m.ID, ReactionEvent{
		MessageID: m.ID.String(), ChannelID: c.ID.String(), ActorID: by.ID.String(),
		MessageAuthorID: m.AuthorID.String(), Emoji: emoji,
	})
}

// withReactions fills Message.Reactions for a page of messages.
// withReplyParticipants fills ReplyParticipantIDs on the thread roots.
func (s *Service) withReplyParticipants(ctx context.Context, msgs []Message) ([]Message, error) {
	var roots []ids.ID
	for _, m := range msgs {
		if m.ReplyCount > 0 {
			roots = append(roots, m.ID)
		}
	}
	if len(roots) == 0 {
		return msgs, nil
	}
	byRoot, err := s.Store.ReplyParticipants(ctx, roots, MaxReplyParticipants)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		msgs[i].ReplyParticipantIDs = byRoot[msgs[i].ID]
	}
	return msgs, nil
}

func (s *Service) withReactions(ctx context.Context, msgs []Message) ([]Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}
	idList := make([]ids.ID, len(msgs))
	for i, m := range msgs {
		idList[i] = m.ID
	}
	rs, err := s.Store.ReactionsFor(ctx, idList)
	if err != nil {
		return nil, err
	}
	byMsg := map[ids.ID][]ReactionSummary{}
	for _, r := range rs {
		list := byMsg[r.MessageID]
		found := false
		for i := range list {
			if list[i].Emoji == r.Emoji {
				list[i].Count++
				list[i].ActorIDs = append(list[i].ActorIDs, r.ActorID)
				found = true
				break
			}
		}
		if !found {
			list = append(list, ReactionSummary{Emoji: r.Emoji, Count: 1, ActorIDs: []ids.ID{r.ActorID}})
		}
		byMsg[r.MessageID] = list
	}
	for i := range msgs {
		msgs[i].Reactions = byMsg[msgs[i].ID]
	}
	s.enrichLinkCards(ctx, msgs)
	return msgs, nil
}
