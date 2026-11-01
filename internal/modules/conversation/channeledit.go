package conversation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/policy"
)

// ChannelUpdate is a change to a channel's name and/or description; nil
// fields stay as they are, an empty description clears it.
type ChannelUpdate struct {
	Name        *string
	Description *string
}

// UpdateChannel renames a channel and/or changes its description (admins
// only). It writes a silent note per changed field; the event carries the
// field names and the names, never the description text.
func (s *Service) UpdateChannel(ctx context.Context, by identity.Actor, channelID ids.ID, in ChannelUpdate) (Channel, error) {
	if !policy.CanManageMembers(subject(by)) {
		return Channel{}, ErrForbidden
	}
	if in.Name == nil && in.Description == nil {
		return Channel{}, ErrNothingToUpdate
	}
	if in.Name != nil {
		if err := ValidateName(*in.Name); err != nil {
			return Channel{}, err
		}
	}
	if in.Description != nil {
		if err := ValidateDescription(*in.Description); err != nil {
			return Channel{}, err
		}
	}
	var out Channel
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
		if err != nil {
			return err
		}
		if c.Kind == KindDM {
			return ErrDMUnchangeable
		}
		ev := ChannelUpdated{ChannelID: c.ID.String(), Fields: []string{}}
		name, desc := c.Name, c.Description
		if in.Name != nil && *in.Name != c.Name {
			name = *in.Name
			ev.Fields = append(ev.Fields, "name")
			ev.OldName, ev.NewName = c.Name, name
		}
		if in.Description != nil && *in.Description != c.Description {
			desc = *in.Description
			ev.Fields = append(ev.Fields, "description")
		}
		out = c
		if len(ev.Fields) == 0 {
			return nil
		}
		if err := s.Store.UpdateChannel(ctx, c.ID, name, desc); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return fmt.Errorf("channel #%s already exists: %w", name, err)
			}
			return err
		}
		out.Name, out.Description = name, desc
		if seq, err = s.appendEvent(ctx, by, c.ID, EventChannelUpdated, "channel", c.ID, ev); err != nil {
			return err
		}
		if ev.NewName != "" {
			if seq, err = s.postChangeNote(ctx, by, out, fmt.Sprintf("✏️ @%s átnevezte: #%s → #%s", by.Handle, ev.OldName, ev.NewName)); err != nil {
				return err
			}
		}
		if desc != c.Description {
			seq, err = s.postChangeNote(ctx, by, out, fmt.Sprintf("📝 @%s frissítette a csatorna leírását", by.Handle))
		}
		return err
	})
	if err != nil {
		return Channel{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return out, nil
}

// ArchiveChannel makes a channel read-only for everyone (admins only).
// Members and history stay; the delivery module acks its pending items and
// the guard stops runs in it on the event.
func (s *Service) ArchiveChannel(ctx context.Context, by identity.Actor, channelID ids.ID) (Channel, error) {
	return s.setArchived(ctx, by, channelID, true)
}

// UnarchiveChannel makes an archived channel writable again (admins only).
func (s *Service) UnarchiveChannel(ctx context.Context, by identity.Actor, channelID ids.ID) (Channel, error) {
	return s.setArchived(ctx, by, channelID, false)
}

func (s *Service) setArchived(ctx context.Context, by identity.Actor, channelID ids.ID, archive bool) (Channel, error) {
	if !policy.CanManageMembers(subject(by)) {
		return Channel{}, ErrForbidden
	}
	var out Channel
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
		if err != nil {
			return err
		}
		if c.Kind == KindDM {
			return ErrDMUnchangeable
		}
		out = c
		if c.Archived() == archive {
			return nil
		}
		typ, note := EventChannelUnarchive, fmt.Sprintf("🗄 @%s visszaállította a csatornát", by.Handle)
		var at *time.Time
		var who *ids.ID
		if archive {
			now, id := s.Clock.Now(), by.ID
			at, who = &now, &id
			typ, note = EventChannelArchived, fmt.Sprintf("🗄 @%s archiválta a csatornát", by.Handle)
		}
		// The note goes in first: once archived, nothing may be written.
		if archive {
			if _, err = s.postChangeNote(ctx, by, c, note); err != nil {
				return err
			}
		}
		if err := s.Store.SetArchived(ctx, c.ID, at, who); err != nil {
			return err
		}
		out.ArchivedAt, out.ArchivedBy = at, who
		if seq, err = s.appendEvent(ctx, by, c.ID, typ, "channel", c.ID, ChannelArchived{ChannelID: c.ID.String(), Name: c.Name}); err != nil {
			return err
		}
		if !archive {
			seq, err = s.postChangeNote(ctx, by, out, note)
		}
		return err
	})
	if err != nil {
		return Channel{}, err
	}
	if seq > 0 {
		s.Bus.Notify(seq)
	}
	return out, nil
}

// postChangeNote writes a silent system note about a channel change inside
// the caller's transaction: it wakes nobody (no delivery) and counts as seen
// for its author.
func (s *Service) postChangeNote(ctx context.Context, by identity.Actor, c Channel, body string) (int64, error) {
	m := Message{
		ID: s.IDs.New(), WorkspaceID: c.WorkspaceID, ChannelID: c.ID, AuthorID: by.ID,
		Kind: "system", Body: body, CreatedAt: s.Clock.Now(),
	}
	if err := s.Store.InsertMessage(ctx, m); err != nil {
		return 0, err
	}
	count, err := s.Store.IncrementMsgCount(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	if _, err := s.Store.Member(ctx, c.ID, by.ID); err == nil {
		if err := s.Store.SetSeen(ctx, c.ID, by.ID, count); err != nil {
			return 0, err
		}
	} else if !errors.Is(err, db.ErrNotFound) {
		return 0, err
	}
	payload := MessageCreatedPayload(m)
	payload.Silent = true
	return s.appendEvent(ctx, by, c.ID, EventMessageCreated, "message", m.ID, payload)
}

// Mute mutes a channel for the caller (humans only). until nil mutes until
// unmuted. Only the caller's own unread display changes; agents and wakes
// are not affected.
func (s *Service) Mute(ctx context.Context, by identity.Actor, channelID ids.ID, until *time.Time) error {
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	now := s.Clock.Now()
	if until != nil && !until.After(now) {
		return ErrBadMuteUntil
	}
	return s.changeMute(ctx, by, channelID, func(ctx context.Context) error {
		return s.Store.SetMute(ctx, Mute{ActorID: by.ID, ChannelID: channelID, Until: until, CreatedAt: now})
	}, MuteChanged{ChannelID: channelID.String(), ActorID: by.ID.String(), Muted: true, MutedUntil: until})
}

// Unmute removes the caller's mute.
func (s *Service) Unmute(ctx context.Context, by identity.Actor, channelID ids.ID) error {
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	return s.changeMute(ctx, by, channelID, func(ctx context.Context) error {
		return s.Store.DeleteMute(ctx, by.ID, channelID)
	}, MuteChanged{ChannelID: channelID.String(), ActorID: by.ID.String()})
}

func (s *Service) changeMute(ctx context.Context, by identity.Actor, channelID ids.ID, write func(context.Context) error, ev MuteChanged) error {
	ok, err := s.CanRead(ctx, by, channelID)
	if err != nil {
		return err
	}
	if !ok {
		return db.ErrNotFound
	}
	var seq int64
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := write(ctx); err != nil {
			return err
		}
		// Not a channel event: the gateway sends it to the viewer only.
		seq, err = s.appendEvent(ctx, by, ids.Nil, EventMuteChanged, "channel", channelID, ev)
		return err
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}

// withMutes fills the viewer's active mutes into the views.
func (s *Service) withMutes(ctx context.Context, actorID ids.ID, views []ChannelView) error {
	mutes, err := s.Store.Mutes(ctx, actorID)
	if err != nil || len(mutes) == 0 {
		return err
	}
	now := s.Clock.Now()
	byChannel := make(map[ids.ID]Mute, len(mutes))
	for _, m := range mutes {
		if m.Active(now) {
			byChannel[m.ChannelID] = m
		}
	}
	for i := range views {
		if m, ok := byChannel[views[i].ID]; ok {
			views[i].Muted, views[i].MutedUntil = true, m.Until
		}
	}
	return nil
}

// GetChannel returns a channel the caller can read, with the caller's unread
// counts and mute; anything else is db.ErrNotFound.
func (s *Service) GetChannel(ctx context.Context, by identity.Actor, channelID ids.ID) (ChannelView, error) {
	ok, err := s.CanRead(ctx, by, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	if !ok {
		return ChannelView{}, db.ErrNotFound
	}
	c, err := s.channelFor(ctx, by.WorkspaceID, channelID)
	if err != nil {
		return ChannelView{}, err
	}
	v := ChannelView{Channel: c}
	if m, err := s.Store.Member(ctx, c.ID, by.ID); err == nil {
		v.IsMember, v.Unread, v.Mentions = true, c.MsgCount-m.MsgCount, m.MentionCount
	} else if !errors.Is(err, db.ErrNotFound) {
		return ChannelView{}, err
	}
	if c.Kind == KindDM {
		if v.MemberIDs, err = s.Store.MemberIDs(ctx, c.ID); err != nil {
			return ChannelView{}, err
		}
	}
	views := []ChannelView{v}
	if err := s.withMutes(ctx, by.ID, views); err != nil {
		return ChannelView{}, err
	}
	return views[0], nil
}

// ChannelInfo is a channel's description and members for the channel_info
// MCP tool.
type ChannelInfo struct {
	Channel Channel
	Members []ChannelMemberView
}

// ChannelInfo returns a channel's details to one of its members; anyone
// else gets db.ErrNotFound, without details. ref is an id or a #name.
func (s *Service) ChannelInfo(ctx context.Context, by identity.Actor, ref string) (ChannelInfo, error) {
	var c Channel
	var err error
	if id, perr := ids.Parse(ref); perr == nil {
		c, err = s.channelFor(ctx, by.WorkspaceID, id)
	} else {
		c, err = s.Store.ChannelByName(ctx, by.WorkspaceID, strings.TrimPrefix(ref, "#"))
	}
	if err != nil {
		return ChannelInfo{}, db.ErrNotFound
	}
	if _, err := s.Store.Member(ctx, c.ID, by.ID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ChannelInfo{}, db.ErrNotFound
		}
		return ChannelInfo{}, err
	}
	members, err := s.ListMembers(ctx, by, c.ID)
	if err != nil {
		return ChannelInfo{}, err
	}
	return ChannelInfo{Channel: c, Members: members}, nil
}
