package delivery

import (
	"context"
	"encoding/json"
	"fmt"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
)

// ConsumerName is the event-log consumer that fills the inbox.
const ConsumerName = "delivery"

// Service holds the delivery use cases.
type Service struct {
	Store        Store
	Conversation conversation.Reader
	Directory    identity.Directory
	IDs          ids.Generator
	Clock        clock.Clock
}

// HandleEvent reacts to events from other modules. It runs inside the
// consumer transaction; inserting is idempotent, so replaying is safe.
func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	switch e.Type {
	case conversation.EventMessageCreated:
		var p conversation.MessageCreated
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delivery: bad %s payload: %w", e.Type, err)
		}
		return s.onMessage(ctx, e, p)
	case conversation.EventReactionAdded:
		var p conversation.ReactionEvent
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delivery: bad %s payload: %w", e.Type, err)
		}
		return s.onReaction(ctx, e, p)
	case question.EventAnswered:
		var p question.Event
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return fmt.Errorf("delivery: bad %s payload: %w", e.Type, err)
		}
		return s.onQuestionAnswered(ctx, e, p)
	}
	return nil
}

// onReaction tells an agent that someone disagreed (👎) with its message.
// Reactions never wake anyone: the item waits for the next wake-up.
func (s *Service) onReaction(ctx context.Context, e eventlog.Event, p conversation.ReactionEvent) error {
	if p.Emoji != conversation.ThumbsDown || p.ActorID == p.MessageAuthorID {
		return nil
	}
	authorID, err := ids.Parse(p.MessageAuthorID)
	if err != nil {
		return err
	}
	msgID, err := ids.Parse(p.MessageID)
	if err != nil {
		return err
	}
	author, err := s.Directory.ActorByID(ctx, authorID)
	if err != nil {
		return err
	}
	if author.Kind != identity.KindAgent {
		return nil
	}
	_, err = s.Store.Insert(ctx, Delivery{
		ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, RecipientID: author.ID, ItemKind: ItemReactionDown,
		ItemID: msgID, ChannelID: e.ChannelID, FromActorID: e.ActorID, Flags: FlagUnread,
		EventSeq: e.Seq, CreatedAt: s.Clock.Now(),
	})
	return err
}

func (s *Service) onQuestionAnswered(ctx context.Context, e eventlog.Event, p question.Event) error {
	askerID, err := ids.Parse(p.AskedBy)
	if err != nil {
		return err
	}
	var msgID ids.ID
	if p.AnswerMessageID != "" {
		if parsed, err := ids.Parse(p.AnswerMessageID); err == nil {
			msgID = parsed
		}
	}
	chID := e.ChannelID
	if chID == ids.Nil && p.ChannelID != "" {
		if parsed, err := ids.Parse(p.ChannelID); err == nil {
			chID = parsed
		}
	}
	asker, err := s.Directory.ActorByID(ctx, askerID)
	if err != nil {
		return err
	}
	flags := FlagUnread
	if asker.Kind == identity.KindAgent {
		flags |= FlagWake
	}
	fromActorID := e.ActorID
	if fromActorID == ids.Nil && p.AnsweredBy != "" {
		if parsed, err := ids.Parse(p.AnsweredBy); err == nil {
			fromActorID = parsed
		}
	}
	_, err = s.Store.Insert(ctx, Delivery{
		ID:          s.IDs.New(),
		WorkspaceID: e.WorkspaceID,
		RecipientID: asker.ID,
		ItemKind:    ItemAnswer,
		ItemID:      msgID,
		ChannelID:   chID,
		FromActorID: fromActorID,
		Flags:       flags,
		EventSeq:    e.Seq,
		CreatedAt:   s.Clock.Now(),
	})
	return err
}

func (s *Service) onMessage(ctx context.Context, e eventlog.Event, p conversation.MessageCreated) error {
	msgID, err := ids.Parse(p.MessageID)
	if err != nil {
		return err
	}
	ch, err := s.Conversation.ChannelByID(ctx, e.ChannelID)
	if err != nil {
		return err
	}
	mentioned := map[string]bool{}
	for _, m := range p.Mentions {
		mentioned[m] = true
	}
	members, err := s.Conversation.MemberIDs(ctx, ch.ID)
	if err != nil {
		return err
	}
	actors, err := s.Directory.ActorsByIDs(ctx, members)
	if err != nil {
		return err
	}
	following, err := s.threadFollowers(ctx, ch, p)
	if err != nil {
		return err
	}
	for _, a := range actors {
		if a.ID.String() == p.AuthorID {
			continue
		}
		var flags Flags
		if mentioned[a.ID.String()] {
			flags |= FlagUnread | FlagMentioned
		}
		if following[a.ID] && a.Kind == identity.KindAgent {
			flags |= FlagUnread | FlagThread
		}
		if ch.Kind == conversation.KindDM {
			flags |= FlagUnread | FlagDM
		}
		if flags == 0 {
			continue // ordinary channel traffic: counters cover it, no inbox row
		}
		if a.Kind == identity.KindAgent {
			flags |= FlagWake
		}
		d := Delivery{
			ID: s.IDs.New(), WorkspaceID: e.WorkspaceID, RecipientID: a.ID, ItemKind: ItemMessage,
			ItemID: msgID, ChannelID: ch.ID, FromActorID: e.ActorID, Flags: flags, EventSeq: e.Seq, CreatedAt: s.Clock.Now(),
		}
		if _, err := s.Store.Insert(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// threadFollowers returns who a reply reaches without a mention: a human's
// reply in a channel thread goes to the agents that wrote in that thread
// (like following a thread in Slack). A reply that mentions someone reaches
// only them, and an agent's reply wakes nobody this way, so two agents in a
// thread cannot keep waking each other.
func (s *Service) threadFollowers(ctx context.Context, ch conversation.Channel, p conversation.MessageCreated) (map[ids.ID]bool, error) {
	if p.ThreadRootID == "" || len(p.Mentions) > 0 || ch.Kind == conversation.KindDM || p.NoThreadFollow {
		return nil, nil
	}
	authorID, err := ids.Parse(p.AuthorID)
	if err != nil {
		return nil, err
	}
	author, err := s.Directory.ActorByID(ctx, authorID)
	if err != nil {
		return nil, err
	}
	if author.Kind != identity.KindHuman {
		return nil, nil
	}
	rootID, err := ids.Parse(p.ThreadRootID)
	if err != nil {
		return nil, err
	}
	authors, err := s.Conversation.ThreadAuthorIDs(ctx, rootID)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.ID]bool, len(authors))
	for _, id := range authors {
		out[id] = true
	}
	return out, nil
}

// Inbox returns the actor's pending items with the message text attached,
// and the total number pending.
func (s *Service) Inbox(ctx context.Context, by identity.Actor, limit int) ([]InboxItem, int, error) {
	if limit <= 0 {
		limit = DefaultInboxLimit
	}
	if limit > MaxInboxLimit {
		limit = MaxInboxLimit
	}
	ds, total, err := s.Store.Pending(ctx, by.ID, limit)
	if err != nil {
		return nil, 0, err
	}
	msgIDs := make([]ids.ID, 0, len(ds))
	for _, d := range ds {
		if d.ItemKind == ItemMessage || d.ItemKind == ItemReactionDown || d.ItemKind == ItemAnswer {
			if d.ItemID != ids.Nil {
				msgIDs = append(msgIDs, d.ItemID)
			}
		}
	}
	msgs, err := s.Conversation.MessagesByIDs(ctx, msgIDs)
	if err != nil {
		return nil, 0, err
	}
	byID := map[ids.ID]conversation.Message{}
	authorIDs := []ids.ID{}
	for _, m := range msgs {
		byID[m.ID] = m
		authorIDs = append(authorIDs, m.AuthorID)
	}
	for _, d := range ds {
		if d.FromActorID != ids.Nil {
			authorIDs = append(authorIDs, d.FromActorID)
		}
	}
	authors, err := s.Directory.ActorsByIDs(ctx, authorIDs)
	if err != nil {
		return nil, 0, err
	}
	handles := map[ids.ID]string{}
	kinds := map[ids.ID]string{}
	for _, a := range authors {
		handles[a.ID] = a.Handle
		kinds[a.ID] = string(a.Kind)
	}
	channelNames := map[ids.ID]string{}
	items := make([]InboxItem, 0, len(ds))
	for _, d := range ds {
		item := InboxItem{Delivery: d}
		if m, ok := byID[d.ItemID]; ok {
			item.Body = m.Body
			item.AuthorHandle = handles[m.AuthorID]
			item.AuthorKind = kinds[m.AuthorID]
			item.ThreadRootID = m.ThreadRootID
			item.MessageAt = m.CreatedAt
		}
		if d.FromActorID != ids.Nil {
			item.AuthorHandle = handles[d.FromActorID]
			item.AuthorKind = kinds[d.FromActorID]
		}
		if item.AuthorHandle == "" && d.ItemKind == ItemAnswer {
			item.AuthorHandle = "system"
			item.AuthorKind = "system"
		}
		if _, ok := channelNames[d.ChannelID]; !ok {
			ch, err := s.Conversation.ChannelByID(ctx, d.ChannelID)
			if err != nil {
				return nil, 0, err
			}
			channelNames[d.ChannelID] = ch.Name
		}
		item.ChannelName = channelNames[d.ChannelID]
		items = append(items, item)
	}
	return items, total, nil
}

// Ack marks inbox items as handled. Unknown or foreign IDs are ignored.
func (s *Service) Ack(ctx context.Context, by identity.Actor, idList []ids.ID) (int, error) {
	return s.Store.Ack(ctx, by.ID, idList, s.Clock.Now())
}
