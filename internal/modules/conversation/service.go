package conversation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/policy"
)

// Attachments handles file attachment validation and loading.
type Attachments interface {
	ValidateAndAttach(ctx context.Context, by identity.Actor, channelID ids.ID, messageID ids.ID, fileIDs []ids.ID) error
	AttachmentsForMessages(ctx context.Context, messageIDs []ids.ID) (map[ids.ID][]Attachment, error)
}

type CompactionReader interface {
	CompactionIDsBySystemMessageIDs(ctx context.Context, messageIDs []ids.ID) (map[ids.ID]ids.ID, error)
}

// Service holds the conversation use cases.
type Service struct {
	Store       Store
	Directory   identity.Directory
	Events      eventlog.Store
	Bus         bus.Bus
	Tx          db.TxRunner
	IDs         ids.Generator
	Clock       clock.Clock
	Attachments Attachments
	LinkCards   LinkCardProvider
	Compactions CompactionReader
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
func (s *Service) CreateChannel(ctx context.Context, by identity.Actor, name, description string, kind ChannelKind) (Channel, error) {
	if !policy.CanCreateChannel(subject(by)) {
		return Channel{}, ErrForbidden
	}
	if err := ValidateName(name); err != nil {
		return Channel{}, err
	}
	if err := ValidateDescription(description); err != nil {
		return Channel{}, err
	}
	if kind == "" {
		kind = KindPublic
	}
	now := s.Clock.Now()
	c := Channel{
		ID:          s.IDs.New(),
		WorkspaceID: by.WorkspaceID,
		Kind:        kind,
		Name:        name,
		Description: description,
		CreatedBy:   by.ID,
		CreatedAt:   now,
	}
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

// ChannelMemberView is a channel member with actor details for clients.
type ChannelMemberView struct {
	ActorID     ids.ID
	Handle      string
	DisplayName string
	Kind        identity.Kind
	CanWrite    bool
}

// ListMembers lists members of channelID. Any actor who can read the channel may list its members.
func (s *Service) ListMembers(ctx context.Context, by identity.Actor, channelID ids.ID) ([]ChannelMemberView, error) {
	if _, err := s.channelFor(ctx, by.WorkspaceID, channelID); err != nil {
		return nil, err
	}
	canRead, err := s.CanRead(ctx, by, channelID)
	if err != nil {
		return nil, err
	}
	if !canRead {
		return nil, ErrForbidden
	}
	members, err := s.Store.ListMembers(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]ChannelMemberView, 0, len(members))
	for _, m := range members {
		act, err := s.Directory.ActorByID(ctx, m.ActorID)
		if err != nil {
			continue
		}
		out = append(out, ChannelMemberView{
			ActorID:     m.ActorID,
			Handle:      act.Handle,
			DisplayName: act.DisplayName,
			Kind:        act.Kind,
			CanWrite:    m.CanWrite,
		})
	}
	return out, nil
}

// RemoveMember removes an actor from a channel or DM for another module
// (archiving an agent), without a note. Only admins may do this.
// Soft-removes the member. Removing the last human admin of a private
// channel is refused.
func (s *Service) RemoveMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID) error {
	return s.removeMember(ctx, by, channelID, actorID, false)
}

// RemoveChannelMember is RemoveMember for the channel editor: a DM's members
// are fixed, and the removal leaves a silent note in the channel.
func (s *Service) RemoveChannelMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID) error {
	return s.removeMember(ctx, by, channelID, actorID, true)
}

func (s *Service) removeMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID, editor bool) error {
	if !policy.CanManageMembers(subject(by)) {
		return ErrForbidden
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
		if err != nil {
			return err
		}
		if c.Kind == KindDM && editor {
			return ErrDMUnchangeable
		}
		target, err := s.Directory.ActorByID(ctx, actorID)
		if err != nil {
			return fmt.Errorf("actor: %w", err)
		}
		if target.WorkspaceID != by.WorkspaceID {
			return fmt.Errorf("actor: %w", db.ErrNotFound)
		}
		if _, err := s.Store.Member(ctx, c.ID, actorID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return ErrMemberNotFound
			}
			return err
		}
		if c.Kind == KindPrivate && target.Kind == identity.KindHuman && target.IsAdmin() {
			members, err := s.Store.ListMembers(ctx, c.ID)
			if err != nil {
				return err
			}
			hasOtherAdmin := false
			for _, m := range members {
				if m.ActorID == target.ID {
					continue
				}
				other, err := s.Directory.ActorByID(ctx, m.ActorID)
				if err != nil {
					continue
				}
				if other.Kind == identity.KindHuman && other.IsAdmin() {
					hasOtherAdmin = true
					break
				}
			}
			if !hasOtherAdmin {
				return ErrLastAdmin
			}
		}

		now := s.Clock.Now()
		if err := s.Store.RemoveMember(ctx, c.ID, actorID, now); err != nil {
			return err
		}
		seq, err = s.appendEvent(ctx, by, c.ID, EventMemberRemoved, "channel", c.ID,
			MemberRemoved{ChannelID: c.ID.String(), ActorID: actorID.String()})
		if err != nil || !editor {
			return err
		}
		seq, err = s.postChangeNote(ctx, by, c, fmt.Sprintf("👋 @%s eltávolította: @%s", by.Handle, target.Handle))
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

// AddMember adds an actor to a channel for another module (a new agent's
// channels, the dev seed), without a note. Only admins may do this. Adding
// an existing member is a no-op.
func (s *Service) AddMember(ctx context.Context, by identity.Actor, channelID, actorID ids.ID, canWrite bool) error {
	return s.addMembers(ctx, by, channelID, []ids.ID{actorID}, canWrite, false)
}

// MaxBatchMembers bounds one batch add.
const MaxBatchMembers = 100

// AddMembers adds actors to a channel in one transaction for the channel
// editor (admins only) and writes one silent note naming the new members.
// Existing members are skipped; a DM's members are fixed.
func (s *Service) AddMembers(ctx context.Context, by identity.Actor, channelID ids.ID, actorIDs []ids.ID, canWrite bool) error {
	return s.addMembers(ctx, by, channelID, actorIDs, canWrite, true)
}

func (s *Service) addMembers(ctx context.Context, by identity.Actor, channelID ids.ID, actorIDs []ids.ID, canWrite, editor bool) error {
	if !policy.CanManageMembers(subject(by)) {
		return ErrForbidden
	}
	if len(actorIDs) == 0 || len(actorIDs) > MaxBatchMembers {
		return ErrMemberNotFound
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
		if err != nil {
			return err
		}
		if c.Kind == KindDM && editor {
			return ErrDMUnchangeable
		}
		// Validate every target before the first write: the memory store
		// cannot roll back.
		targets := make([]identity.Actor, 0, len(actorIDs))
		seen := map[ids.ID]bool{}
		for _, id := range actorIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			target, err := s.Directory.ActorByID(ctx, id)
			if err != nil {
				return fmt.Errorf("actor: %w", err)
			}
			if target.WorkspaceID != by.WorkspaceID {
				return fmt.Errorf("actor: %w", db.ErrNotFound)
			}
			targets = append(targets, target)
		}
		var added []string
		for _, target := range targets {
			n, err := s.addMember(ctx, by, c, target.ID, canWrite)
			if err != nil {
				return err
			}
			if n > 0 {
				seq = n
				added = append(added, "@"+target.Handle)
			}
		}
		if len(added) == 0 || !editor {
			return nil
		}
		seq, err = s.postChangeNote(ctx, by, c, fmt.Sprintf("➕ @%s hozzáadta: %s", by.Handle, strings.Join(added, ", ")))
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
	FileIDs      []ids.ID
	// Elaborate lets an agent exceed AgentMaxBody; set only when someone
	// asked for a detailed answer.
	Elaborate bool
	// NoThreadFollow keeps a thread reply from reaching the thread's agents
	// as a plain reply; set when the sender delivers it another way (a
	// question's answer reaches the asker as its own answer item).
	NoThreadFollow bool
	// Silent posts the message without delivery to any actor (no inbox items, no wake).
	Silent bool
	// OriginChannelID and OriginThreadID record where an agent request came from.
	// Set by agents through MCP send when target differs from origin.
	OriginChannelID ids.ID
	OriginThreadID  ids.ID
	// AnswersMessageID links an agent's answer to the message it responds
	// to in another channel; kept only if the agent can read that message.
	AnswersMessageID ids.ID
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
	if in.ClientMsgID != "" {
		existing, err := s.Store.MessageByClientID(ctx, by.ID, in.ClientMsgID)
		if err == nil {
			if s.Attachments != nil {
				atts, err := s.Attachments.AttachmentsForMessages(ctx, []ids.ID{existing.ID})
				if err == nil {
					existing.Attachments = atts[existing.ID]
				}
			}
			msgs := []Message{existing}
			s.enrichLinkCards(ctx, msgs)
			s.enrichOrigins(ctx, by, msgs)
			return SendResult{Message: &msgs[0]}, nil
		}
		if !errors.Is(err, db.ErrNotFound) {
			return SendResult{}, err
		}
	}
	c, err := s.channelFor(ctx, by.WorkspaceID, in.ChannelID)
	if err != nil {
		return SendResult{}, err
	}
	if c.Archived() {
		return SendResult{}, ErrChannelArchived
	}
	mem, err := s.membership(ctx, c.ID, by.ID)
	if err != nil {
		return SendResult{}, err
	}
	if !policy.CanWriteChannel(subject(by), mem) {
		return SendResult{}, ErrForbidden
	}
	if in.ThreadRootID != ids.Nil {
		root, err := s.Store.MessageByID(ctx, in.ThreadRootID)
		if err != nil || root.ChannelID != c.ID || root.ThreadRootID != ids.Nil {
			return SendResult{}, ErrBadThread
		}
	}
	if by.Kind == identity.KindAgent {
		if err := s.checkAndEnforceRateLimit(ctx, by, c, in.ThreadRootID); err != nil {
			return SendResult{}, err
		}
		if err := checkAgentMessage(body, len(MentionedHandles(body)), in.Elaborate); err != nil {
			return SendResult{}, err
		}
	}
	var originChannelID, originThreadID ids.ID
	if by.Kind == identity.KindAgent && in.OriginChannelID != ids.Nil && in.OriginThreadID != ids.Nil {
		// Store origin only when target differs from origin.
		if c.ID != in.OriginChannelID || in.ThreadRootID != in.OriginThreadID {
			// Ignore origin if agent is not a member of the origin channel or thread does not exist.
			if origMem, err := s.membership(ctx, in.OriginChannelID, by.ID); err == nil && origMem.IsMember {
				if origThread, err := s.Store.MessageByID(ctx, in.OriginThreadID); err == nil && origThread.ChannelID == in.OriginChannelID {
					originChannelID = in.OriginChannelID
					originThreadID = in.OriginThreadID
				}
			}
		}
	}
	var answersID ids.ID
	if by.Kind == identity.KindAgent && in.AnswersMessageID != ids.Nil && in.ThreadRootID != ids.Nil {
		answersID = s.checkedAnswers(ctx, by, c.ID, in.ThreadRootID, in.AnswersMessageID)
	}
	var msg Message
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if in.ClientMsgID != "" {
			existing, err := s.Store.MessageByClientID(ctx, by.ID, in.ClientMsgID)
			if err == nil {
				if s.Attachments != nil {
					atts, err := s.Attachments.AttachmentsForMessages(ctx, []ids.ID{existing.ID})
					if err == nil {
						existing.Attachments = atts[existing.ID]
					}
				}
				msg = existing
				return nil
			}
			if !errors.Is(err, db.ErrNotFound) {
				return err
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
			OriginChannelID: originChannelID, OriginThreadID: originThreadID,
			AnswersMessageID: answersID,
		}
		if err := s.Store.InsertMessage(ctx, msg); err != nil {
			return err
		}
		if len(in.FileIDs) > 0 {
			if s.Attachments == nil {
				return errors.New("file attachments not supported")
			}
			if err := s.Attachments.ValidateAndAttach(ctx, by, c.ID, msg.ID, in.FileIDs); err != nil {
				return err
			}
			atts, err := s.Attachments.AttachmentsForMessages(ctx, []ids.ID{msg.ID})
			if err != nil {
				return err
			}
			msg.Attachments = atts[msg.ID]
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
		payload := MessageCreatedPayload(msg)
		payload.NoThreadFollow = in.NoThreadFollow
		payload.Silent = in.Silent
		seq, err = s.appendEvent(ctx, by, c.ID, EventMessageCreated, "message", msg.ID, payload)
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
	s.enrichOrigins(ctx, by, msgs)
	msg = msgs[0]
	return SendResult{Message: &msg}, nil
}

// checkedAnswers returns refID only if it is another actor's reply, in a DM
// the agent belongs to, to the agent's own request that was sent from exactly
// this channel and thread (the reply-routing case). Anything else is dropped,
// so an agent cannot attach a link to an unrelated message.
func (s *Service) checkedAnswers(ctx context.Context, by identity.Actor, channelID, threadRootID, refID ids.ID) ids.ID {
	ref, err := s.Store.MessageByID(ctx, refID)
	if err != nil || ref.DeletedAt != nil || ref.AuthorID == by.ID {
		return ids.Nil
	}
	refCh, err := s.Store.ChannelByID(ctx, ref.ChannelID)
	if err != nil || refCh.Kind != KindDM {
		return ids.Nil
	}
	if mem, err := s.membership(ctx, ref.ChannelID, by.ID); err != nil || !mem.IsMember {
		return ids.Nil
	}
	var req Message
	if ref.ThreadRootID != ids.Nil {
		req, err = s.Store.MessageByID(ctx, ref.ThreadRootID)
	} else {
		req, err = s.Store.LatestTopLevelMessageByAuthor(ctx, ref.ChannelID, by.ID)
		if err == nil && req.CreatedAt.After(ref.CreatedAt) {
			return ids.Nil
		}
	}
	if err != nil || req.AuthorID != by.ID || req.OriginChannelID != channelID || req.OriginThreadID != threadRootID {
		return ids.Nil
	}
	return ref.ID
}

// enrichOrigins fills the origin channel name and the answered message for
// the viewer, and hides both when the viewer cannot read that channel: a DM
// reader must not learn about a private channel, nor a channel reader about
// a DM.
func (s *Service) enrichOrigins(ctx context.Context, by identity.Actor, msgs []Message) {
	readable := map[ids.ID]bool{}
	canRead := func(channelID ids.ID) bool {
		if ok, seen := readable[channelID]; seen {
			return ok
		}
		ok, err := s.CanRead(ctx, by, channelID)
		readable[channelID] = err == nil && ok
		return readable[channelID]
	}
	for i := range msgs {
		if msgs[i].AnswersMessageID != ids.Nil {
			ref, err := s.Store.MessageByID(ctx, msgs[i].AnswersMessageID)
			if err == nil && ref.DeletedAt == nil && canRead(ref.ChannelID) {
				msgs[i].AnswersChannelID = ref.ChannelID
				msgs[i].AnswersAuthorID = ref.AuthorID
			} else {
				msgs[i].AnswersMessageID = ids.Nil
			}
		}
		if msgs[i].OriginChannelID != ids.Nil {
			ch, err := s.Store.ChannelByID(ctx, msgs[i].OriginChannelID)
			if err == nil && canRead(ch.ID) {
				msgs[i].OriginChannel = ch.Name
			} else {
				msgs[i].OriginChannelID = ids.Nil
				msgs[i].OriginThreadID = ids.Nil
			}
		}
	}
}

// LatestTopLevelMessageByAuthor returns the newest top-level message in the channel by the author.
func (s *Service) LatestTopLevelMessageByAuthor(ctx context.Context, channelID, authorID ids.ID) (Message, error) {
	return s.Store.LatestTopLevelMessageByAuthor(ctx, channelID, authorID)
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
		c, err := s.channelFor(ctx, by.WorkspaceID, in.ChannelID)
		if err != nil {
			return err
		}
		if c.Archived() {
			return ErrChannelArchived
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
		Mentions:    make([]string, 0, len(m.Mentions)),
		Attachments: m.Attachments,
	}
	if m.ThreadRootID != ids.Nil {
		p.ThreadRootID = m.ThreadRootID.String()
	}
	for _, id := range m.Mentions {
		p.Mentions = append(p.Mentions, id.String())
	}
	return p
}

// ListChannels lists the channels the actor may see, with unread counts and
// the actor's own mutes. Archived channels are left out unless an admin asks
// for them.
func (s *Service) ListChannels(ctx context.Context, by identity.Actor, archived bool) ([]ChannelView, error) {
	all, err := s.Store.ChannelsForActor(ctx, by.WorkspaceID, by.ID, by.IsAdmin())
	if err != nil {
		return nil, err
	}
	archived = archived && by.IsAdmin()
	views := all[:0]
	for _, v := range all {
		if !v.Archived() || archived {
			views = append(views, v)
		}
	}
	if err := s.withMutes(ctx, by.ID, views); err != nil {
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
	msgs, err = s.withReplyParticipants(ctx, msgs)
	if err != nil {
		return nil, err
	}
	msgs, err = s.withReactions(ctx, msgs)
	if err != nil {
		return nil, err
	}
	msgs, err = s.withAttachments(ctx, msgs)
	if err != nil {
		return nil, err
	}
	s.enrichOrigins(ctx, by, msgs)
	return s.withCompactions(ctx, maskRetracted(msgs))
}

// withAttachments fills Message.Attachments for a slice of messages.
func (s *Service) withAttachments(ctx context.Context, msgs []Message) ([]Message, error) {
	if len(msgs) == 0 || s.Attachments == nil {
		return msgs, nil
	}
	idList := make([]ids.ID, len(msgs))
	for i, m := range msgs {
		idList[i] = m.ID
	}
	byMsg, err := s.Attachments.AttachmentsForMessages(ctx, idList)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		if atts, ok := byMsg[msgs[i].ID]; ok {
			msgs[i].Attachments = atts
		}
	}
	return msgs, nil
}

func (s *Service) withCompactions(ctx context.Context, msgs []Message) ([]Message, error) {
	if len(msgs) == 0 || s.Compactions == nil {
		return msgs, nil
	}
	idList := make([]ids.ID, len(msgs))
	for i, m := range msgs {
		idList[i] = m.ID
	}
	byMsg, err := s.Compactions.CompactionIDsBySystemMessageIDs(ctx, idList)
	if err != nil {
		return msgs, nil
	}
	for i := range msgs {
		if cid, ok := byMsg[msgs[i].ID]; ok {
			msgs[i].CompactionID = cid
		}
	}
	return msgs, nil
}

// MessagesInUnit returns messages in a unit (thread or main stream), oldest first,
// excluding retracted messages.
func (s *Service) MessagesInUnit(ctx context.Context, channelID, threadRootID ids.ID) ([]Message, error) {
	var msgs []Message
	var err error
	if threadRootID != ids.Nil {
		msgs, err = s.Store.ThreadMessages(ctx, threadRootID)
	} else {
		msgs, err = s.Store.MainStreamMessages(ctx, channelID)
	}
	if err != nil {
		return nil, err
	}
	active := msgs[:0]
	for _, m := range msgs {
		if m.DeletedAt == nil {
			active = append(active, m)
		}
	}
	msgs = active
	msgs, err = s.withAttachments(ctx, msgs)
	if err != nil {
		return nil, err
	}
	return s.withCompactions(ctx, msgs)
}

// PostSystemMessage posts a system message to a unit with optional file attachments.
func (s *Service) PostSystemMessage(ctx context.Context, workspaceID, channelID, threadRootID, authorID ids.ID, body string, fileIDs []ids.ID) (Message, error) {
	now := s.Clock.Now()
	sysMsg := Message{
		ID:           s.IDs.New(),
		WorkspaceID:  workspaceID,
		ChannelID:    channelID,
		ThreadRootID: threadRootID,
		AuthorID:     authorID,
		Kind:         "system",
		Body:         body,
		CreatedAt:    now,
	}
	err := s.Tx.InTx(ctx, func(txCtx context.Context) error {
		if err := s.Store.InsertMessage(txCtx, sysMsg); err != nil {
			return err
		}
		if len(fileIDs) > 0 && s.Attachments != nil {
			by := identity.Actor{ID: authorID, WorkspaceID: workspaceID}
			if err := s.Attachments.ValidateAndAttach(txCtx, by, channelID, sysMsg.ID, fileIDs); err != nil {
				return err
			}
		}
		if _, err := s.Store.IncrementMsgCount(txCtx, channelID); err != nil {
			return err
		}
		if threadRootID != ids.Nil {
			if err := s.Store.BumpThread(txCtx, threadRootID, now); err != nil {
				return err
			}
		}
		by, err := s.Directory.ActorByID(txCtx, authorID)
		if err != nil {
			by = identity.Actor{ID: authorID, WorkspaceID: workspaceID}
		}
		_, err = s.appendEvent(txCtx, by, channelID, EventMessageCreated, "message", sysMsg.ID, MessageCreatedPayload(sysMsg))
		return err
	})
	if err != nil {
		return Message{}, err
	}
	return sysMsg, nil
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

// CanWrite implements Reader.
func (s *Service) CanWrite(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error) {
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
	return policy.CanWriteChannel(subject(by), mem), nil
}

// MessageByID implements Reader.
func (s *Service) MessageByID(ctx context.Context, id ids.ID) (Message, error) {
	m, err := s.Store.MessageByID(ctx, id)
	if err != nil {
		return Message{}, err
	}
	return maskRetracted([]Message{m})[0], nil
}

// MessagesByIDs implements Reader.
func (s *Service) MessagesByIDs(ctx context.Context, idList []ids.ID) ([]Message, error) {
	msgs, err := s.Store.MessagesByIDs(ctx, idList)
	if err != nil {
		return nil, err
	}
	return maskRetracted(msgs), nil
}

// MemberIDs implements Reader.
func (s *Service) MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error) {
	return s.Store.MemberIDs(ctx, channelID)
}

// ChannelInWorkspace reports whether a channel exists in the workspace.
func (s *Service) ChannelInWorkspace(ctx context.Context, workspaceID, channelID ids.ID) (bool, error) {
	c, err := s.Store.ChannelByID(ctx, channelID)
	if errors.Is(err, db.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return c.WorkspaceID == workspaceID, nil
}

// ChannelIDsOf lists the channels (and DMs) an actor belongs to, in the
// caller's workspace (archiving an agent removes its memberships).
func (s *Service) ChannelIDsOf(ctx context.Context, by identity.Actor, actorID ids.ID) ([]ids.ID, error) {
	views, err := s.Store.ChannelsForActor(ctx, by.WorkspaceID, actorID, false)
	if err != nil {
		return nil, err
	}
	out := make([]ids.ID, len(views))
	for i, v := range views {
		out[i] = v.ID
	}
	return out, nil
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

func (s *Service) checkAndEnforceRateLimit(ctx context.Context, by identity.Actor, c Channel, threadRootID ids.ID) error {
	if by.Kind != identity.KindAgent {
		return nil
	}
	if threadRootID == ids.Nil && c.Kind != KindDM {
		return nil
	}

	paused, alreadyNotified, err := s.isConversationPaused(ctx, c, threadRootID)
	if err != nil {
		return err
	}
	if !paused {
		return nil
	}

	if !alreadyNotified {
		if err := s.postPauseSystemNote(ctx, by, c, threadRootID); err != nil {
			return err
		}
	}

	return ErrConversationPaused
}

func (s *Service) isConversationPaused(ctx context.Context, c Channel, threadRootID ids.ID) (bool, bool, error) {
	var msgs []Message
	var err error
	if threadRootID != ids.Nil {
		msgs, err = s.Store.ThreadMessages(ctx, threadRootID)
	} else {
		msgs, err = s.Store.ListMessages(ctx, c.ID, ids.Nil, 100)
	}
	if err != nil {
		return false, false, err
	}

	now := s.Clock.Now()
	cutoff := now.Add(-AgentRateLimitWindow)

	// Collect unique author IDs of non-system messages
	authorIDs := make(map[ids.ID]bool)
	for _, m := range msgs {
		if m.Kind != "system" {
			authorIDs[m.AuthorID] = true
		}
	}
	actorKindMap := make(map[ids.ID]identity.Kind)
	if s.Directory != nil && len(authorIDs) > 0 {
		var idList []ids.ID
		for id := range authorIDs {
			idList = append(idList, id)
		}
		actors, err := s.Directory.ActorsByIDs(ctx, idList)
		if err == nil {
			for _, a := range actors {
				actorKindMap[a.ID] = a.Kind
			}
		}
	}

	agentCount := 0
	alreadyNotified := false
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]

		// Check if a system pause note was already posted in this pause
		if m.Kind == "system" && strings.Contains(strings.ToLower(m.Body), "paused") {
			alreadyNotified = true
			continue
		}

		kind, ok := actorKindMap[m.AuthorID]
		if !ok && s.Directory != nil {
			if a, err := s.Directory.ActorByID(ctx, m.AuthorID); err == nil {
				kind = a.Kind
				actorKindMap[m.AuthorID] = kind
				ok = true
			}
		}

		// Any human message lifts the pause
		if ok && kind == identity.KindHuman {
			break
		}

		// If message is older than the rate limit window, stop counting agent messages
		if m.CreatedAt.Before(cutoff) {
			break
		}

		if ok && kind == identity.KindAgent {
			agentCount++
		}
	}

	if agentCount >= AgentRateLimitCount {
		return true, alreadyNotified, nil
	}
	return false, false, nil
}

func (s *Service) postPauseSystemNote(ctx context.Context, by identity.Actor, c Channel, threadRootID ids.ID) error {
	var adminHandles []string
	var adminIDs []ids.ID
	if s.Directory != nil {
		admins, err := s.Directory.Admins(ctx, by.WorkspaceID)
		if err == nil {
			for _, a := range admins {
				if a.Kind == identity.KindHuman {
					adminHandles = append(adminHandles, a.Handle)
					adminIDs = append(adminIDs, a.ID)
				}
			}
			sort.Strings(adminHandles)
		}
	}

	var mentionStr string
	if len(adminHandles) > 0 {
		var parts []string
		for _, h := range adminHandles {
			parts = append(parts, "@"+h)
		}
		mentionStr = " " + strings.Join(parts, ", ")
	}
	body := fmt.Sprintf("⚠️ Conversation paused: at least %d agent messages were posted in the last 10 minutes without a human reply.%s please check in.", AgentRateLimitCount, mentionStr)

	now := s.Clock.Now()
	sysMsg := Message{
		ID:           s.IDs.New(),
		WorkspaceID:  by.WorkspaceID,
		ChannelID:    c.ID,
		ThreadRootID: threadRootID,
		AuthorID:     by.ID,
		Kind:         "system",
		Body:         body,
		Mentions:     adminIDs,
		CreatedAt:    now,
	}

	var seq int64
	err := s.Tx.InTx(ctx, func(txCtx context.Context) error {
		// Another request may have posted the note meanwhile. Look only at
		// the current pause (back to the last human message): a note from an
		// earlier pause, lifted by a human, must not silence this one.
		if _, notified, err := s.isConversationPaused(txCtx, c, threadRootID); err != nil {
			return err
		} else if notified {
			return nil
		}

		if err := s.Store.InsertMessage(txCtx, sysMsg); err != nil {
			return err
		}
		if _, err := s.Store.IncrementMsgCount(txCtx, c.ID); err != nil {
			return err
		}
		if len(adminIDs) > 0 {
			if err := s.Store.IncrementMentions(txCtx, c.ID, adminIDs); err != nil {
				return err
			}
		}
		if threadRootID != ids.Nil {
			if err := s.Store.BumpThread(txCtx, threadRootID, now); err != nil {
				return err
			}
		}
		evSeq, err := s.appendEvent(txCtx, by, c.ID, EventMessageCreated, "message", sysMsg.ID, MessageCreatedPayload(sysMsg))
		if err != nil {
			return err
		}
		seq = evSeq
		return nil
	})
	if err != nil {
		return err
	}
	if seq > 0 && s.Bus != nil {
		s.Bus.Notify(seq)
	}
	return nil
}

// ThreadAuthorIDs implements Reader.
func (s *Service) ThreadAuthorIDs(ctx context.Context, rootID ids.ID) ([]ids.ID, error) {
	msgs, err := s.Store.ThreadMessages(ctx, rootID)
	if err != nil {
		return nil, err
	}
	seen := map[ids.ID]bool{}
	var out []ids.ID
	for _, m := range msgs {
		if !seen[m.AuthorID] {
			seen[m.AuthorID] = true
			out = append(out, m.AuthorID)
		}
	}
	return out, nil
}

// RetractMessage allows a human author to retract their own message.
func (s *Service) RetractMessage(ctx context.Context, by identity.Actor, messageID ids.ID) error {
	if by.Halted {
		return ErrForbidden
	}
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	m, err := s.Store.MessageByID(ctx, messageID)
	if err != nil {
		return err
	}
	if m.AuthorID != by.ID {
		return ErrForbidden
	}
	if m.DeletedAt != nil {
		return nil
	}
	now := s.Clock.Now()
	payload := MessageRetracted{
		MessageID:    m.ID.String(),
		ChannelID:    m.ChannelID.String(),
		ThreadRootID: m.ThreadRootID.String(),
		AuthorID:     m.AuthorID.String(),
		RetractedBy:  by.ID.String(),
		At:           now,
	}
	var seq int64
	err = s.Tx.InTx(ctx, func(txCtx context.Context) error {
		if err := s.Store.RetractMessage(txCtx, m.ID, by.ID, now); err != nil {
			return err
		}
		var err error
		seq, err = s.appendEvent(txCtx, by, m.ChannelID, EventMessageRetracted, "message", m.ID, payload)
		return err
	})
	if err != nil {
		return err
	}
	if seq > 0 && s.Bus != nil {
		s.Bus.Notify(seq)
	}
	return nil
}

// maskRetracted drops what a retracted message said, for every reader
// (the page, the API, agents' MCP tools); the row itself stays unchanged.
func maskRetracted(msgs []Message) []Message {
	for i := range msgs {
		if msgs[i].DeletedAt != nil {
			msgs[i].Body = ""
			msgs[i].Mentions = nil
			msgs[i].Reactions = nil
			msgs[i].Attachments = nil
			msgs[i].LinkCards = nil
		}
	}
	return msgs
}
