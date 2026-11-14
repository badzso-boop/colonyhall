// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package activity

import (
	"context"
	"encoding/json"
	"errors"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/approval"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/question"
	"colonyhall/internal/modules/saved"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// ConsumerName is the event consumer that builds the feed.
const ConsumerName = "activity"

// Conversation is what the feed needs from the conversation module.
type Conversation interface {
	ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error)
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
	MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error)
	ThreadAuthorIDs(ctx context.Context, rootID ids.ID) ([]ids.ID, error)
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	NotifyLevelFor(ctx context.Context, actorID ids.ID, c conversation.Channel) (conversation.NotifyLevel, error)
}

// Service holds the feed's use cases and its consumer.
type Service struct {
	Store        Store
	Conversation Conversation
	Directory    identity.Directory
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// View is an item with what its recipient may still see of its message.
type View struct {
	Item
	Message *conversation.Message
	Channel *conversation.Channel
}

// List returns a page of the caller's feed, newest first, the unread count,
// and the next cursor.
func (s *Service) List(ctx context.Context, by identity.Actor, q Query) ([]View, int, *Cursor, error) {
	if by.Kind != identity.KindHuman {
		return nil, 0, nil, ErrForbidden
	}
	if q.Kind != "" && !ValidKind(q.Kind) {
		return nil, 0, nil, ErrBadKind
	}
	if q.Limit <= 0 {
		q.Limit = PageSize
	}
	if q.Limit > MaxPageSize {
		q.Limit = MaxPageSize
	}
	limit := q.Limit
	q.Limit++
	items, err := s.Store.List(ctx, by.WorkspaceID, by.ID, q)
	if err != nil {
		return nil, 0, nil, err
	}
	var next *Cursor
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = &Cursor{At: last.CreatedAt, ID: last.ID}
	}
	unread, err := s.Store.CountUnread(ctx, by.WorkspaceID, by.ID)
	if err != nil {
		return nil, 0, nil, err
	}
	readable := map[ids.ID]bool{}
	out := make([]View, 0, len(items))
	for _, it := range items {
		v := View{Item: it}
		if it.ChannelID != ids.Nil {
			ok, seen := readable[it.ChannelID]
			if !seen {
				if ok, err = s.Conversation.CanRead(ctx, by, it.ChannelID); err != nil {
					return nil, 0, nil, err
				}
				readable[it.ChannelID] = ok
			}
			if ok {
				if c, err := s.Conversation.ChannelByID(ctx, it.ChannelID); err == nil {
					v.Channel = &c
				}
				if it.MessageID != ids.Nil {
					if m, err := s.Conversation.MessageByID(ctx, it.MessageID); err == nil {
						v.Message = &m
					}
				}
			}
		}
		out = append(out, v)
	}
	return out, unread, next, nil
}

// MarkRead marks the caller's items read: the given ones, every one in a
// channel, or all of them.
func (s *Service) MarkRead(ctx context.Context, by identity.Actor, itemIDs []ids.ID, channelID ids.ID) (int, error) {
	if by.Kind != identity.KindHuman {
		return 0, ErrForbidden
	}
	var n int
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if n, err = s.Store.MarkRead(ctx, by.ID, itemIDs, channelID, s.Clock.Now()); err != nil || n == 0 {
			return err
		}
		ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventRead, by.ID, ids.Nil, "activity", by.ID, Read{Marked: n}, s.Clock.Now())
		if err != nil {
			return err
		}
		if err := s.Events.Append(ctx, &ev); err != nil {
			return err
		}
		seq = ev.Seq
		return nil
	})
	if err != nil {
		return 0, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return n, nil
}

// HandleEvent turns an event into feed items for the people it concerns.
// It runs in the consumer's transaction, in the event's workspace scope.
func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	switch e.Type {
	case conversation.EventMessageCreated:
		return s.onMessage(ctx, e)
	case conversation.EventReactionAdded:
		var p conversation.ReactionEvent
		if json.Unmarshal(e.Payload, &p) != nil || p.ActorID == p.MessageAuthorID {
			return nil
		}
		return s.add(ctx, e, KindReaction, parse(p.MessageAuthorID), true, Item{ChannelID: parse(p.ChannelID),
			MessageID: parse(p.MessageID), ActorID: parse(p.ActorID), Emoji: p.Emoji})
	case conversation.EventMessagePinned:
		var p conversation.PinEvent
		if json.Unmarshal(e.Payload, &p) != nil || !p.Important {
			return nil
		}
		members, err := s.Conversation.MemberIDs(ctx, parse(p.ChannelID))
		if err != nil {
			return err
		}
		for _, id := range members {
			if id == parse(p.MessageAuthorID) {
				continue
			}
			if err := s.add(ctx, e, KindImportant, id, true, Item{ChannelID: parse(p.ChannelID), MessageID: parse(p.MessageID),
				ThreadRootID: parse(p.ThreadRootID), ActorID: parse(p.MessageAuthorID)}); err != nil {
				return err
			}
		}
	case approval.EventApprovalRequested:
		var p approval.ApprovalRequested
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		return s.onApproval(ctx, e, p)
	case question.EventAsked:
		var p question.Event
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		return s.onQuestion(ctx, e, p)
	case saved.EventReminded:
		var p saved.Reminded
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		return s.add(ctx, e, KindReminder, e.ActorID, false, Item{ChannelID: parse(p.ReminderChannel),
			MessageID: parse(p.ReminderMessage), RefID: parse(p.ItemID)})
	}
	return nil
}

func (s *Service) onMessage(ctx context.Context, e eventlog.Event) error {
	var p conversation.MessageCreated
	if json.Unmarshal(e.Payload, &p) != nil || p.Silent {
		return nil
	}
	channel, author, msg, root := parse(p.ChannelID), parse(p.AuthorID), parse(p.MessageID), parse(p.ThreadRootID)
	done := map[ids.ID]bool{author: true}
	base := Item{ChannelID: channel, MessageID: msg, ThreadRootID: root, ActorID: author}
	for _, m := range p.Mentions {
		id := parse(m)
		if done[id] {
			continue
		}
		done[id] = true
		if err := s.add(ctx, e, KindMention, id, true, base); err != nil {
			return err
		}
	}
	if root == ids.Nil || p.Kind != "text" {
		return nil
	}
	authors, err := s.Conversation.ThreadAuthorIDs(ctx, root)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil
		}
		return err
	}
	for _, id := range authors {
		if done[id] {
			continue
		}
		done[id] = true
		if err := s.add(ctx, e, KindThreadReply, id, true, base); err != nil {
			return err
		}
	}
	return nil
}

// onApproval: the people who may decide (they write in the channel and
// see the agent) get an item.
func (s *Service) onApproval(ctx context.Context, e eventlog.Event, p approval.ApprovalRequested) error {
	channel, agentID := parse(p.ChannelID), parse(p.AgentID)
	agent, err := s.Directory.ActorByID(ctx, agentID)
	if err != nil {
		return nil
	}
	members, err := s.Conversation.MemberIDs(ctx, channel)
	if err != nil {
		return err
	}
	for _, id := range members {
		a, err := s.Directory.ActorByID(ctx, id)
		if err != nil || a.Kind != identity.KindHuman || !a.SeesAgent(agent) {
			continue
		}
		if ok, err := s.Conversation.CanWrite(ctx, a, channel); err != nil || !ok {
			continue
		}
		if err := s.add(ctx, e, KindApproval, id, false, Item{ChannelID: channel, ActorID: agentID, RefID: parse(p.ApprovalID)}); err != nil {
			return err
		}
	}
	return nil
}

// onQuestion: a question to one person goes to them; one to "a human" to
// the people in its channel.
func (s *Service) onQuestion(ctx context.Context, e eventlog.Event, p question.Event) error {
	channel := parse(p.ChannelID)
	it := Item{ChannelID: channel, ThreadRootID: parse(p.ThreadRootID), ActorID: parse(p.AskedBy), RefID: parse(p.QuestionID)}
	if to := parse(p.ToActorID); to != ids.Nil {
		return s.add(ctx, e, KindQuestion, to, false, it)
	}
	if !p.ToHuman {
		return nil
	}
	members, err := s.Conversation.MemberIDs(ctx, channel)
	if err != nil {
		return err
	}
	for _, id := range members {
		if id == it.ActorID {
			continue
		}
		if err := s.add(ctx, e, KindQuestion, id, false, it); err != nil {
			return err
		}
	}
	return nil
}

// add stores an item for a person who can still read the channel; byLevel
// items are left out where the person chose to hear nothing (round 55, I).
func (s *Service) add(ctx context.Context, e eventlog.Event, kind Kind, recipientID ids.ID, byLevel bool, it Item) error {
	if recipientID == ids.Nil {
		return nil
	}
	who, err := s.Directory.ActorByID(ctx, recipientID)
	if err != nil || who.Kind != identity.KindHuman || who.WorkspaceID != e.WorkspaceID {
		return nil
	}
	if it.ChannelID != ids.Nil {
		ok, err := s.Conversation.CanRead(ctx, who, it.ChannelID)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if byLevel {
			c, err := s.Conversation.ChannelByID(ctx, it.ChannelID)
			if err != nil {
				return err
			}
			level, err := s.Conversation.NotifyLevelFor(ctx, recipientID, c)
			if err != nil {
				return err
			}
			if level == conversation.NotifyNone {
				return nil
			}
		}
	}
	it.ID, it.WorkspaceID, it.RecipientID, it.Kind, it.SourceSeq, it.CreatedAt = s.IDs.New(), e.WorkspaceID, recipientID, kind, e.Seq, e.At
	added, err := s.Store.Insert(ctx, it)
	if err != nil || !added {
		return err
	}
	// The badge signal goes to the recipient alone (the gateway's personal rule).
	ev, err := eventlog.NewEvent(s.IDs.New(), e.WorkspaceID, EventCreated, recipientID, ids.Nil, "activity", it.ID,
		Created{ItemID: it.ID.String(), Kind: kind}, s.Clock.Now())
	if err != nil {
		return err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return err
	}
	s.Bus.Notify(ev.Seq)
	return nil
}

func parse(s string) ids.ID {
	id, err := ids.Parse(s)
	if err != nil {
		return ids.Nil
	}
	return id
}
