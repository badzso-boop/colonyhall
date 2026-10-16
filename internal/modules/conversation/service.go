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
	LinkCards LinkCardProvider
}

func subject(a identity.Actor) policy.Subject {
	return policy.Subject{IsAdmin: a.IsAdmin(), IsAgent: a.Kind == identity.KindAgent, Halted: a.Halted}
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
		c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
		if err != nil {
			return err
		}
		target, err := s.Directory.ActorByID(ctx, actorID)
		if err != nil {
			return fmt.Errorf("actor: %w", err)
		}
		if target.WorkspaceID != by.WorkspaceID {
			return fmt.Errorf("actor: %w", db.ErrNotFound)
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

// EnsureMember ensures that actorID is a member of channelID with write permission.
func (s *Service) EnsureMember(ctx context.Context, channelID, actorID ids.ID) error {
	m, err := s.Store.Member(ctx, channelID, actorID)
	if err == nil {
		if !m.CanWrite {
			// already a member, write is permitted
			return nil
		}
		return nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return err
	}
	mem := Member{
		ChannelID: channelID,
		ActorID:   actorID,
		CanWrite:  true,
		JoinedAt:  s.Clock.Now(),
	}
	return s.Store.AddMember(ctx, mem)
}

// SendInput is a message to send.
type SendInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	ClientMsgID  string
	Body         string
	// Elaborate lets an agent exceed AgentMaxBody; set only when someone
	// asked for a detailed answer.
	Elaborate bool
}

// SendResult is the outcome of SendMessage: either a posted message, or
// (for an agent's bare acknowledgement) the 👍 reaction it became.
type SendResult struct {
	Message             *Message
	ConvertedToReaction *Reaction
}

// SendMessage posts a message. Sending again with the same ClientMsgID
// returns the original message instead of creating a duplicate. For agents
// the shared agent rules apply: bare acknowledgements become a 👍 reaction,
// and length and mention limits are enforced.
func (s *Service) SendMessage(ctx context.Context, by identity.Actor, in SendInput) (SendResult, error) {
	if by.Halted {
		return SendResult{}, identity.ErrHalted
	}
	body, err := ValidateBody(in.Body)
	if err != nil {
		return SendResult{}, err
	}
	if len(in.ClientMsgID) > MaxClientMsgID {
		return SendResult{}, ErrInvalidClientID
	}
	if by.Kind == identity.KindAgent && IsAcknowledgement(body) {
		return s.ackAsReaction(ctx, by, in)
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
		c, err := s.channelFor(ctx, by.WorkspaceID, in.ChannelID)
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
		if by.Kind == identity.KindAgent {
			if err := checkAgentMessage(body, len(MentionedHandles(body)), in.Elaborate); err != nil {
				return err
			}
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
		return SendResult{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	msgs := []Message{msg}
	s.enrichLinkCards(ctx, msgs)
	msg = msgs[0]
	return SendResult{Message: &msg}, nil
}

func (s *Service) enrichLinkCards(ctx context.Context, msgs []Message) {
	if s.LinkCards == nil || len(msgs) == 0 {
		return
	}
	cardsMap, err := s.LinkCards.LinkCardsForMessages(ctx, msgs)
	if err != nil {
		return
	}
	for i := range msgs {
		msgs[i].LinkCards = cardsMap[msgs[i].ID]
	}
}

// ackAsReaction turns an agent's "ok"/"thanks" into a 👍 on the message it
// answers: the thread root, or else the newest message by someone else.
func (s *Service) ackAsReaction(ctx context.Context, by identity.Actor, in SendInput) (SendResult, error) {
	var target ids.ID
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.channelFor(ctx, by.WorkspaceID, in.ChannelID); err != nil {
			return err
		}
		target = in.ThreadRootID
		if target == ids.Nil {
			last, err := s.Store.LastMessageNotBy(ctx, in.ChannelID, by.ID)
			if errors.Is(err, db.ErrNotFound) {
				return ErrNothingToAck
			}
			if err != nil {
				return err
			}
			target = last.ID
		}
		var err error
		seq, err = s.reactInTx(ctx, by, target, ThumbsUp, true)
		return err
	})
	if err != nil {
		return SendResult{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return SendResult{ConvertedToReaction: &Reaction{MessageID: target, ActorID: by.ID, Emoji: ThumbsUp}}, nil
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
	views, err := s.Store.ChannelsForActor(ctx, by.WorkspaceID, by.ID, by.IsAdmin())
	if err != nil {
		return nil, err
	}
	for i := range views {
		if views[i].Kind == KindDM {
			if views[i].MemberIDs, err = s.Store.MemberIDs(ctx, views[i].ID); err != nil {
				return nil, err
			}
		}
	}
	return views, nil
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
	ok, err := s.CanRead(ctx, by, c.ID)
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
	ok, err := s.CanRead(ctx, by, channelID)
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
	msgs, err := s.Store.ListMessages(ctx, channelID, before, limit)
	if err != nil {
		return nil, err
	}
	return s.withReactions(ctx, msgs)
}

// MarkRead marks every message in the channel as seen by the actor.
func (s *Service) MarkRead(ctx context.Context, by identity.Actor, channelID ids.ID) error {
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
		if err != nil {
			return err
		}
		if _, err := s.Store.Member(ctx, channelID, by.ID); err != nil {
			return err
		}
		return s.Store.SetSeen(ctx, channelID, by.ID, c.MsgCount)
	})
}

// channelFor loads a channel and hides it unless it belongs to the caller's
// workspace, so IDs from another workspace behave as if they did not exist.
func (s *Service) channelFor(ctx context.Context, workspaceID, channelID ids.ID) (Channel, error) {
	c, err := s.Store.ChannelByID(ctx, channelID)
	if err != nil {
		return Channel{}, err
	}
	if c.WorkspaceID != workspaceID {
		return Channel{}, db.ErrNotFound
	}
	return c, nil
}

// CanRead implements Reader. Channels outside the actor's workspace are
// never readable, not even by an admin.
func (s *Service) CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error) {
	if _, err := s.channelFor(ctx, by.WorkspaceID, channelID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	mem, err := s.membership(ctx, channelID, by.ID)
	if err != nil {
		return false, err
	}
	return policy.CanReadChannel(subject(by), mem), nil
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
