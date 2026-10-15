package conversation

import (
	"context"
	"errors"
	"fmt"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/policy"
)

// Service holds the conversation use cases.
type Service struct {
	Store     Store
	Directory identity.Directory
	Events    eventlog.Store
	Bus       bus.Bus
	Tx        db.TxRunner
	IDs       ids.Generator
	Clock     clock.Clock
}

func subject(a identity.Actor) policy.Subject {
	return policy.Subject{IsAdmin: a.IsAdmin(), IsAgent: a.Kind == identity.KindAgent}
}

func (s *Service) membership(ctx context.Context, channelID, actorID ids.ID) (policy.Membership, error) {
	m, err := s.Store.Member(ctx, channelID, actorID)
	if errors.Is(err, db.ErrNotFound) {
		return policy.Membership{}, nil
	}
	if err != nil {
		return policy.Membership{}, err
	}
	return policy.Membership{IsMember: true, CanWrite: m.CanWrite}, nil
}

// CreateChannel creates a channel; the creator becomes its first member.
func (s *Service) CreateChannel(ctx context.Context, by identity.Actor, name string, kind ChannelKind) (Channel, error) {
	if !policy.CanCreateChannel(subject(by)) {
		return Channel{}, ErrForbidden
	}
	if err := ValidateName(name); err != nil {
		return Channel{}, err
	}
	if kind == "" {
		kind = KindPublic
	}
	now := s.Clock.Now()
	c := Channel{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, Kind: kind, Name: name, CreatedBy: by.ID, CreatedAt: now}
	var lastSeq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateChannel(ctx, c); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return fmt.Errorf("channel #%s already exists: %w", name, err)
			}
			return err
		}
		seq, err := s.appendEvent(ctx, by, c.ID, EventChannelCreated, "channel", c.ID,
			ChannelCreated{ChannelID: c.ID.String(), Kind: kind, Name: name})
		if err != nil {
			return err
		}
		lastSeq = seq
		seq, err = s.addMember(ctx, by, c, by.ID, true)
		if err != nil {
			return err
		}
		lastSeq = seq
		return nil
	})
	if err != nil {
		return Channel{}, err
	}
	s.Bus.Notify(lastSeq)
	return c, nil
}

// AddMember adds an actor to a channel. Only admins may do this.
// Adding an existing member is a no-op.
func (s *Service) AddMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID, canWrite bool) error {
	if !policy.CanManageMembers(subject(by)) {
		return ErrForbidden
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.ChannelByID(ctx, channelID)
		if err != nil {
			return err
		}
		if _, err := s.Directory.ActorByID(ctx, actorID); err != nil {
			return fmt.Errorf("actor: %w", err)
		}
		seq, err = s.addMember(ctx, by, c, actorID, canWrite)
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

func (s *Service) addMember(ctx context.Context, by identity.Actor, c Channel, actorID ids.ID, canWrite bool) (int64, error) {
	if _, err := s.Store.Member(ctx, c.ID, actorID); err == nil {
		return 0, nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return 0, err
	}
	m := Member{ChannelID: c.ID, ActorID: actorID, CanWrite: canWrite, MsgCount: c.MsgCount, JoinedAt: s.Clock.Now()}
	if err := s.Store.AddMember(ctx, m); err != nil {
		return 0, err
	}
	return s.appendEvent(ctx, by, c.ID, EventMemberAdded, "channel", c.ID,
		MemberAdded{ChannelID: c.ID.String(), ActorID: actorID.String(), CanWrite: canWrite})
}

// SendInput is a message to send.
type SendInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	ClientMsgID  string
	Body         string
}

// SendMessage posts a message. Sending again with the same ClientMsgID
// returns the original message instead of creating a duplicate.
func (s *Service) SendMessage(ctx context.Context, by identity.Actor, in SendInput) (Message, error) {
	body, err := ValidateBody(in.Body)
	if err != nil {
		return Message{}, err
	}
	if len(in.ClientMsgID) > MaxClientMsgID {
		return Message{}, ErrInvalidClientID
	}
	var msg Message
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if in.ClientMsgID != "" {
			existing, err := s.Store.MessageByClientID(ctx, by.ID, in.ClientMsgID)
			if err == nil {
				msg = existing
				return nil
			}
			if !errors.Is(err, db.ErrNotFound) {
				return err
			}
		}
		c, err := s.Store.ChannelByID(ctx, in.ChannelID)
		if err != nil {
			return err
		}
		mem, err := s.membership(ctx, c.ID, by.ID)
		if err != nil {
			return err
		}
		if !policy.CanWriteChannel(subject(by), mem) {
			return ErrForbidden
		}
		if in.ThreadRootID != ids.Nil {
			root, err := s.Store.MessageByID(ctx, in.ThreadRootID)
			if err != nil || root.ChannelID != c.ID || root.ThreadRootID != ids.Nil {
				return ErrBadThread
			}
		}
		mentions, err := s.resolveMentions(ctx, by, c.ID, body)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		msg = Message{
			ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, ChannelID: c.ID, ThreadRootID: in.ThreadRootID,
			AuthorID: by.ID, ClientMsgID: in.ClientMsgID, Kind: "text", Body: body,
			Mentions: mentions, CreatedAt: now,
		}
		if err := s.Store.InsertMessage(ctx, msg); err != nil {
			return err
		}
		count, err := s.Store.IncrementMsgCount(ctx, c.ID)
		if err != nil {
			return err
		}
		if mem.IsMember {
			if err := s.Store.SetSeen(ctx, c.ID, by.ID, count); err != nil {
				return err
			}
		}
		if err := s.Store.IncrementMentions(ctx, c.ID, mentions); err != nil {
			return err
		}
		if in.ThreadRootID != ids.Nil {
			if err := s.Store.BumpThread(ctx, in.ThreadRootID, now); err != nil {
				return err
			}
		}
		seq, err = s.appendEvent(ctx, by, c.ID, EventMessageCreated, "message", msg.ID, MessageCreatedPayload(msg))
		return err
	})
	if err != nil {
		return Message{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return msg, nil
}

// resolveMentions keeps the @handles that are members of the channel,
// excluding the author.
func (s *Service) resolveMentions(ctx context.Context, by identity.Actor, channelID ids.ID, body string) ([]ids.ID, error) {
	var out []ids.ID
	for _, h := range MentionedHandles(body) {
		a, err := s.Directory.ActorByHandle(ctx, by.WorkspaceID, h)
		if errors.Is(err, db.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if a.ID == by.ID {
			continue
		}
		if _, err := s.Store.Member(ctx, channelID, a.ID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, a.ID)
	}
	return out, nil
}

// MessageCreatedPayload builds the event payload for a message.
func MessageCreatedPayload(m Message) MessageCreated {
	p := MessageCreated{
		MessageID: m.ID.String(), ChannelID: m.ChannelID.String(), AuthorID: m.AuthorID.String(),
		ClientMsgID: m.ClientMsgID, Kind: m.Kind, Body: m.Body, CreatedAt: m.CreatedAt,
		Mentions: make([]string, 0, len(m.Mentions)),
	}
	if m.ThreadRootID != ids.Nil {
		p.ThreadRootID = m.ThreadRootID.String()
	}
	for _, id := range m.Mentions {
		p.Mentions = append(p.Mentions, id.String())
	}
	return p
}

// ListChannels lists the channels the actor may see, with unread counts.
func (s *Service) ListChannels(ctx context.Context, by identity.Actor) ([]ChannelView, error) {
	return s.Store.ChannelsForActor(ctx, by.WorkspaceID, by.ID, by.IsAdmin())
}

// ResolveChannel finds a channel by name (with or without '#') and checks read access.
func (s *Service) ResolveChannel(ctx context.Context, by identity.Actor, name string) (Channel, error) {
	if len(name) > 0 && name[0] == '#' {
		name = name[1:]
	}
	c, err := s.Store.ChannelByName(ctx, by.WorkspaceID, name)
	if err != nil {
		return Channel{}, err
	}
	ok, err := s.CanRead(ctx, by.ID, by.IsAdmin(), c.ID)
	if err != nil {
		return Channel{}, err
	}
	if !ok {
		return Channel{}, ErrForbidden
	}
	return c, nil
}

// ListMessages returns a page of messages, oldest first.
func (s *Service) ListMessages(ctx context.Context, by identity.Actor, channelID, before ids.ID, limit int) ([]Message, error) {
	ok, err := s.CanRead(ctx, by.ID, by.IsAdmin(), channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	return s.Store.ListMessages(ctx, channelID, before, limit)
}

// MarkRead marks every message in the channel as seen by the actor.
func (s *Service) MarkRead(ctx context.Context, by identity.Actor, channelID ids.ID) error {
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.Store.ChannelByID(ctx, channelID)
		if err != nil {
			return err
		}
		if _, err := s.Store.Member(ctx, channelID, by.ID); err != nil {
			return err
		}
		return s.Store.SetSeen(ctx, channelID, by.ID, c.MsgCount)
	})
}

// CanRead implements Reader.
func (s *Service) CanRead(ctx context.Context, actorID ids.ID, isAdmin bool, channelID ids.ID) (bool, error) {
	if isAdmin {
		return true, nil
	}
	mem, err := s.membership(ctx, channelID, actorID)
	if err != nil {
		return false, err
	}
	return policy.CanReadChannel(policy.Subject{IsAdmin: isAdmin}, mem), nil
}

// MessagesByIDs implements Reader.
func (s *Service) MessagesByIDs(ctx context.Context, idList []ids.ID) ([]Message, error) {
	return s.Store.MessagesByIDs(ctx, idList)
}

// MemberIDs implements Reader.
func (s *Service) MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error) {
	return s.Store.MemberIDs(ctx, channelID)
}

// ChannelByID implements Reader.
func (s *Service) ChannelByID(ctx context.Context, id ids.ID) (Channel, error) {
	return s.Store.ChannelByID(ctx, id)
}

func (s *Service) appendEvent(ctx context.Context, by identity.Actor, channelID ids.ID, typ, aggType string, aggID ids.ID, payload any) (int64, error) {
	ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, typ, by.ID, channelID, aggType, aggID, payload, s.Clock.Now())
	if err != nil {
		return 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}
