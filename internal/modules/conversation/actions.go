// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package conversation

import (
	"context"

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
