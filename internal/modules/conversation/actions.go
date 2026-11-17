// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"context"
	"strings"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// RecordAction records a person's click or choice on an interactive
// message they can read (round 56c) and emits EventMessageAction; what
// follows (a routine, a reply to the agent) reacts to the event.
func (s *Service) RecordAction(ctx context.Context, by identity.Actor, messageID ids.ID, actionID, value string) (Action, string, error) {
	if by.Halted {
		return Action{}, "", identity.ErrHalted
	}
	if by.Kind != identity.KindHuman {
		return Action{}, "", ErrForbidden
	}
	var a Action
	var label string
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		m, err := s.Store.MessageByID(ctx, messageID)
		if err != nil || m.WorkspaceID != by.WorkspaceID || m.DeletedAt != nil {
			return db.ErrNotFound
		}
		if ok, err := s.CanRead(ctx, by, m.ChannelID); err != nil || !ok {
			return db.ErrNotFound
		}
		l, ok := FindAction(m.Blocks, actionID, value)
		if !ok {
			return ErrUnknownAction
		}
		label = l
		a = Action{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, MessageID: m.ID, ActorID: by.ID, ActionID: actionID,
			Value: value, CreatedAt: s.Clock.Now()}
		if err := s.Store.InsertAction(ctx, a); err != nil {
			return err
		}
		p := MessageAction{ActionRecordID: a.ID.String(), MessageID: m.ID.String(), ChannelID: m.ChannelID.String(),
			MessageAuthorID: m.AuthorID.String(), ActorID: by.ID.String(), ActionID: actionID, Value: value, Label: label}
		if m.ThreadRootID != ids.Nil {
			p.ThreadRootID = m.ThreadRootID.String()
		}
		seq, err = s.appendEvent(ctx, by, m.ChannelID, EventMessageAction, "message", m.ID, p)
		return err
	})
	if err != nil {
		return Action{}, "", err
	}
	s.Bus.Notify(seq)
	return a, label, nil
}

// ChannelIDByName finds a live channel by name for an admin (the Workshop
// puts an extension's bot into its routines' channels, round 56c).
func (s *Service) ChannelIDByName(ctx context.Context, by identity.Actor, name string) (ids.ID, error) {
	if !by.IsAdmin() || by.Kind != identity.KindHuman {
		return ids.Nil, ErrForbidden
	}
	c, err := s.Store.ChannelByName(ctx, by.WorkspaceID, strings.TrimPrefix(strings.TrimSpace(name), "#"))
	if err != nil || c.ArchivedAt != nil || c.Kind == KindDM {
		return ids.Nil, db.ErrNotFound
	}
	return c.ID, nil
}

// IsMember reports whether an actor is a member of a channel.
func (s *Service) IsMember(ctx context.Context, channelID, actorID ids.ID) bool {
	_, err := s.Store.Member(ctx, channelID, actorID)
	return err == nil
}
