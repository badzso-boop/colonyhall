// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"context"
	"errors"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/policy"
)

// Pin events (round 55, part D). Both are channel events: everyone who
// reads the conversation sees them, and the audit log records them.
const (
	EventMessagePinned   = "message.pinned"
	EventMessageUnpinned = "message.unpinned"
)

// PinEvent is the payload of the pin events.
type PinEvent struct {
	MessageID       string `json:"message_id"`
	ChannelID       string `json:"channel_id"`
	ThreadRootID    string `json:"thread_root_id,omitempty"`
	ActorID         string `json:"actor_id"`
	MessageAuthorID string `json:"message_author_id"`
	// Important: the agent sent the message marked important.
	Important bool `json:"important,omitempty"`
}

// agentPinWindow is the period AgentPinsPerDay counts over.
const agentPinWindow = 24 * time.Hour

// Pin pins a message in its conversation. Members who can write there may
// pin; an agent only its own messages, at most AgentPinsPerDay a day per
// conversation. Pinning a pinned message is a no-op.
func (s *Service) Pin(ctx context.Context, by identity.Actor, messageID ids.ID) error {
	if by.Halted {
		return identity.ErrHalted
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		m, c, err := s.pinTarget(ctx, by, messageID)
		if err != nil {
			return err
		}
		seq, err = s.pinInTx(ctx, by, c, m)
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

// Unpin removes a message's pin. Members who can write there may unpin;
// agents may not (they only pin their own results).
func (s *Service) Unpin(ctx context.Context, by identity.Actor, messageID ids.ID) error {
	if by.Halted {
		return identity.ErrHalted
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		m, c, err := s.pinTarget(ctx, by, messageID)
		if err != nil {
			return err
		}
		if by.Kind == identity.KindAgent {
			return ErrForbidden
		}
		changed, err := s.Store.Unpin(ctx, m.ID, by.ID, s.Clock.Now())
		if err != nil || !changed {
			return err
		}
		seq, err = s.appendEvent(ctx, by, c.ID, EventMessageUnpinned, "message", m.ID, pinEvent(by, m))
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

// pinTarget loads the message and checks that the caller may change its
// pin: write access first (an outsider learns nothing), then the state.
func (s *Service) pinTarget(ctx context.Context, by identity.Actor, messageID ids.ID) (Message, Channel, error) {
	m, err := s.Store.MessageByID(ctx, messageID)
	if err != nil {
		return Message{}, Channel{}, err
	}
	c, err := s.channelFor(ctx, by.WorkspaceID, m.ChannelID)
	if err != nil {
		return Message{}, Channel{}, err
	}
	mem, err := s.membership(ctx, c.ID, by.ID)
	if err != nil {
		return Message{}, Channel{}, err
	}
	if !policy.CanReadChannel(subject(by), mem) {
		return Message{}, Channel{}, db.ErrNotFound
	}
	if !policy.CanWriteChannel(subject(by), mem) {
		return Message{}, Channel{}, ErrForbidden
	}
	if c.Archived() {
		return Message{}, Channel{}, ErrChannelArchived
	}
	if by.Kind == identity.KindAgent && c.AgentLocked() {
		return Message{}, Channel{}, ErrChannelLocked
	}
	if m.DeletedAt != nil {
		return Message{}, Channel{}, ErrPinRetracted
	}
	return m, c, nil
}

// pinInTx pins m inside the caller's transaction and returns the event seq
// (0 when it was pinned already).
func (s *Service) pinInTx(ctx context.Context, by identity.Actor, c Channel, m Message) (int64, error) {
	if by.Kind == identity.KindAgent {
		if m.AuthorID != by.ID {
			return 0, ErrPinOwnOnly
		}
		if err := s.checkAgentPinLimit(ctx, by, c.ID); err != nil {
			return 0, err
		}
	}
	if pinned, err := s.Store.PinnedAmong(ctx, []ids.ID{m.ID}); err != nil {
		return 0, err
	} else if pinned[m.ID] {
		return 0, nil
	}
	if n, err := s.Store.CountActivePins(ctx, c.ID); err != nil {
		return 0, err
	} else if n >= MaxPins {
		return 0, ErrPinLimit
	}
	p := Pin{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, ChannelID: c.ID, MessageID: m.ID, PinnedBy: by.ID, PinnedAt: s.Clock.Now()}
	if err := s.Store.InsertPin(ctx, p); err != nil {
		if errors.Is(err, db.ErrConflict) {
			return 0, nil
		}
		return 0, err
	}
	return s.appendEvent(ctx, by, c.ID, EventMessagePinned, "message", m.ID, pinEvent(by, m))
}

func (s *Service) checkAgentPinLimit(ctx context.Context, by identity.Actor, channelID ids.ID) error {
	n, err := s.Store.CountPinsBy(ctx, channelID, by.ID, s.Clock.Now().Add(-agentPinWindow))
	if err != nil {
		return err
	}
	if n >= AgentPinsPerDay {
		return ErrAgentPinLimit
	}
	return nil
}

func pinEvent(by identity.Actor, m Message) PinEvent {
	p := PinEvent{MessageID: m.ID.String(), ChannelID: m.ChannelID.String(), ActorID: by.ID.String(),
		MessageAuthorID: m.AuthorID.String(), Important: m.Important}
	if m.ThreadRootID != ids.Nil {
		p.ThreadRootID = m.ThreadRootID.String()
	}
	return p
}

// ListPins returns the conversation's pinned messages, newest pin first,
// to anyone who can read it.
func (s *Service) ListPins(ctx context.Context, by identity.Actor, channelID ids.ID) ([]PinnedMessage, error) {
	ok, err := s.CanRead(ctx, by, channelID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, db.ErrNotFound
	}
	pins, err := s.Store.ActivePins(ctx, channelID)
	if err != nil || len(pins) == 0 {
		return nil, err
	}
	idList := make([]ids.ID, len(pins))
	for i, p := range pins {
		idList[i] = p.MessageID
	}
	msgs, err := s.Store.MessagesByIDs(ctx, idList)
	if err != nil {
		return nil, err
	}
	if msgs, err = s.withReactions(ctx, msgs); err != nil {
		return nil, err
	}
	if msgs, err = s.withAttachments(ctx, msgs); err != nil {
		return nil, err
	}
	s.enrichOrigins(ctx, by, msgs)
	msgs = maskRetracted(msgs)
	byID := make(map[ids.ID]Message, len(msgs))
	for _, m := range msgs {
		m.Pinned = true
		byID[m.ID] = m
	}
	out := make([]PinnedMessage, 0, len(pins))
	for _, p := range pins {
		if m, ok := byID[p.MessageID]; ok && m.DeletedAt == nil {
			out = append(out, PinnedMessage{Pin: p, Message: m})
		}
	}
	return out, nil
}

// withPins marks the pinned messages of a page.
func (s *Service) withPins(ctx context.Context, msgs []Message) ([]Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}
	idList := make([]ids.ID, len(msgs))
	for i, m := range msgs {
		idList[i] = m.ID
	}
	pinned, err := s.Store.PinnedAmong(ctx, idList)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		msgs[i].Pinned = pinned[msgs[i].ID]
	}
	return msgs, nil
}
