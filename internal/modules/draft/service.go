// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package draft

import (
	"context"
	"encoding/json"
	"strings"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/files"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// ConsumerName is the event consumer that clears a draft once its place
// gets a message from its owner (sending from another device).
const ConsumerName = "draft"

// Conversation is what drafts need from the conversation module.
type Conversation interface {
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
}

// Files is what drafts need from the files module.
type Files interface {
	Get(ctx context.Context, by identity.Actor, fileID ids.ID) (files.File, error)
}

// Service holds the draft use cases.
type Service struct {
	Store        Store
	Conversation Conversation
	Files        Files
	Events       eventlog.Store
	Bus          bus.Bus
	Tx           db.TxRunner
	IDs          ids.Generator
	Clock        clock.Clock
}

// SaveInput is a draft as the composer has it.
type SaveInput struct {
	ChannelID    ids.ID
	ThreadRootID ids.ID
	Body         string
	FileIDs      []ids.ID
}

// place checks that the caller may keep a draft in the channel and thread.
// A channel the caller cannot read is not found.
func (s *Service) place(ctx context.Context, by identity.Actor, channelID, threadRootID ids.ID) error {
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	ok, err := s.Conversation.CanRead(ctx, by, channelID)
	if err != nil {
		return err
	}
	if !ok {
		return db.ErrNotFound
	}
	if threadRootID != ids.Nil {
		root, err := s.Conversation.MessageByID(ctx, threadRootID)
		if err != nil || root.ChannelID != channelID || root.ThreadRootID != ids.Nil {
			return ErrBadThread
		}
	}
	return nil
}

// Save stores the caller's draft in one place; an empty one (no text, no
// files) deletes it. The result is nil after a delete.
func (s *Service) Save(ctx context.Context, by identity.Actor, in SaveInput) (*Draft, error) {
	if err := s.place(ctx, by, in.ChannelID, in.ThreadRootID); err != nil {
		return nil, err
	}
	if len(in.Body) > MaxBodyLen {
		return nil, ErrBodyTooLong
	}
	if len(in.FileIDs) > MaxFiles {
		return nil, ErrTooManyFiles
	}
	fileIDs := dedupe(in.FileIDs)
	if strings.TrimSpace(in.Body) == "" && len(fileIDs) == 0 {
		return nil, s.Delete(ctx, by, in.ChannelID, in.ThreadRootID)
	}
	atts, err := s.attachments(ctx, by, in.ChannelID, fileIDs, true)
	if err != nil {
		return nil, err
	}
	d := Draft{
		ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, ActorID: by.ID, ChannelID: in.ChannelID,
		ThreadRootID: in.ThreadRootID, Body: in.Body, FileIDs: fileIDs, UpdatedAt: s.Clock.Now(),
	}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		saved, err := s.Store.Upsert(ctx, d)
		if err != nil {
			return err
		}
		d = saved
		seq, err = s.appendChanged(ctx, by.WorkspaceID, by.ID, d.ChannelID, d.ThreadRootID, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.Bus.Notify(seq)
	d.Attachments = atts
	return &d, nil
}

// Delete removes the caller's draft in one place; a missing one is fine.
func (s *Service) Delete(ctx context.Context, by identity.Actor, channelID, threadRootID ids.ID) error {
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		gone, err := s.Store.Delete(ctx, by.ID, channelID, threadRootID, nil)
		if err != nil || !gone {
			return err
		}
		seq, err = s.appendChanged(ctx, by.WorkspaceID, by.ID, channelID, threadRootID, true)
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

// List returns the caller's drafts in conversations the caller can still
// read, newest first, with the uploads still usable there.
func (s *Service) List(ctx context.Context, by identity.Actor) ([]Draft, error) {
	if by.Kind != identity.KindHuman {
		return nil, ErrForbidden
	}
	all, err := s.Store.List(ctx, by.WorkspaceID, by.ID, MaxDrafts)
	if err != nil {
		return nil, err
	}
	readable := map[ids.ID]bool{}
	out := make([]Draft, 0, len(all))
	for _, d := range all {
		ok, seen := readable[d.ChannelID]
		if !seen {
			ok, err = s.Conversation.CanRead(ctx, by, d.ChannelID)
			if err != nil {
				return nil, err
			}
			readable[d.ChannelID] = ok
		}
		if !ok {
			continue
		}
		if d.Attachments, err = s.attachments(ctx, by, d.ChannelID, d.FileIDs, false); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// attachments resolves upload IDs to the caller's own files in the channel.
// strict refuses anything else (a save); otherwise such files are left out
// (a read: a file may have been removed since).
func (s *Service) attachments(ctx context.Context, by identity.Actor, channelID ids.ID, fileIDs []ids.ID, strict bool) ([]Attachment, error) {
	out := make([]Attachment, 0, len(fileIDs))
	for _, id := range fileIDs {
		if s.Files == nil {
			if strict {
				return nil, ErrBadFile
			}
			continue
		}
		f, err := s.Files.Get(ctx, by, id)
		if err == nil && (f.UploadedBy != by.ID || f.ChannelID != channelID) {
			err = ErrBadFile
		}
		if err != nil {
			if strict {
				return nil, ErrBadFile
			}
			continue
		}
		out = append(out, Attachment{ID: f.ID, Name: f.Name, Mime: f.Mime, Size: f.Size})
	}
	return out, nil
}

// HandleEvent clears the author's draft where a message of theirs was
// posted (a send from another tab or device), unless the draft was saved
// after that message: the person may already be typing the next one.
func (s *Service) HandleEvent(ctx context.Context, e eventlog.Event) error {
	if e.Type != conversation.EventMessageCreated {
		return nil
	}
	var p conversation.MessageCreated
	if err := json.Unmarshal(e.Payload, &p); err != nil || p.Kind != "text" {
		return nil
	}
	author, err1 := ids.Parse(p.AuthorID)
	channel, err2 := ids.Parse(p.ChannelID)
	if err1 != nil || err2 != nil {
		return nil
	}
	thread := ids.Nil
	if p.ThreadRootID != "" {
		if thread, err1 = ids.Parse(p.ThreadRootID); err1 != nil {
			return nil
		}
	}
	at := p.CreatedAt
	gone, err := s.Store.Delete(ctx, author, channel, thread, &at)
	if err != nil || !gone {
		return err
	}
	seq, err := s.appendChanged(ctx, e.WorkspaceID, author, channel, thread, true)
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}

func (s *Service) appendChanged(ctx context.Context, workspaceID, actorID, channelID, threadRootID ids.ID, deleted bool) (int64, error) {
	p := Changed{ChannelID: channelID.String(), Deleted: deleted}
	if threadRootID != ids.Nil {
		p.ThreadRootID = threadRootID.String()
	}
	// Not a channel event: the gateway sends it to the owner only.
	ev, err := eventlog.NewEvent(s.IDs.New(), workspaceID, EventChanged, actorID, ids.Nil, "draft", channelID, p, s.Clock.Now())
	if err != nil {
		return 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}

func dedupe(in []ids.ID) []ids.ID {
	seen := map[ids.ID]bool{}
	out := make([]ids.ID, 0, len(in))
	for _, id := range in {
		if id != ids.Nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
