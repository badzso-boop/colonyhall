package delivery

import (
	"context"
	"encoding/json"
	"fmt"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
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
	if e.Type != conversation.EventMessageCreated {
		return nil
	}
	var p conversation.MessageCreated
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return fmt.Errorf("delivery: bad %s payload: %w", e.Type, err)
	}
	return s.onMessage(ctx, e, p)
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
	for _, a := range actors {
		if a.ID.String() == p.AuthorID {
			continue
		}
		var flags Flags
		if mentioned[a.ID.String()] {
			flags |= FlagUnread | FlagMentioned
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
			ItemID: msgID, ChannelID: ch.ID, Flags: flags, EventSeq: e.Seq, CreatedAt: s.Clock.Now(),
		}
		if _, err := s.Store.Insert(ctx, d); err != nil {
			return err
		}
	}
	return nil
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
		if d.ItemKind == ItemMessage {
			msgIDs = append(msgIDs, d.ItemID)
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
	authors, err := s.Directory.ActorsByIDs(ctx, authorIDs)
	if err != nil {
		return nil, 0, err
	}
	handles := map[ids.ID]string{}
	for _, a := range authors {
		handles[a.ID] = a.Handle
	}
	channelNames := map[ids.ID]string{}
	items := make([]InboxItem, 0, len(ds))
	for _, d := range ds {
		item := InboxItem{Delivery: d}
		if m, ok := byID[d.ItemID]; ok {
			item.Body = m.Body
			item.AuthorHandle = handles[m.AuthorID]
			item.ThreadRootID = m.ThreadRootID
			item.MessageAt = m.CreatedAt
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
