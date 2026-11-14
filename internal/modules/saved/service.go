package saved

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/eventlog"
	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Conversation is what saved items need from the conversation module.
type Conversation interface {
	MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error)
	ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error)
	CanRead(ctx context.Context, by identity.Actor, channelID ids.ID) (bool, error)
	SelfDM(ctx context.Context, by identity.Actor) (conversation.Channel, error)
	PostSystemMessage(ctx context.Context, workspaceID, channelID, threadRootID, authorID ids.ID, body string, fileIDs []ids.ID) (conversation.Message, error)
}

// Service holds the saved-item and reminder use cases.
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

// SaveInput saves a message, sets a reminder, or both. A message saved
// again is updated (and no longer done).
type SaveInput struct {
	MessageID ids.ID
	RemindAt  *time.Time
	Note      string
}

// View is an item with what the owner may still see of its message.
type View struct {
	Item
	// Message and Channel are set when the owner can still read them.
	Message *conversation.Message
	Channel *conversation.Channel
}

func (s *Service) checkRemindAt(at *time.Time) error {
	if at == nil {
		return nil
	}
	now := s.Clock.Now()
	if !at.After(now) || at.After(now.Add(MaxAhead)) {
		return ErrBadRemindAt
	}
	return nil
}

// Save saves a message (any message the person can read) and/or sets a
// reminder. Humans only.
func (s *Service) Save(ctx context.Context, by identity.Actor, in SaveInput) (Item, error) {
	if by.Kind != identity.KindHuman {
		return Item{}, ErrForbidden
	}
	in.Note = strings.TrimSpace(in.Note)
	if utf8.RuneCountInString(in.Note) > MaxNote {
		return Item{}, ErrNoteTooLong
	}
	if in.MessageID == ids.Nil && in.RemindAt == nil {
		return Item{}, ErrEmpty
	}
	if err := s.checkRemindAt(in.RemindAt); err != nil {
		return Item{}, err
	}
	now := s.Clock.Now()
	it := Item{ID: s.IDs.New(), WorkspaceID: by.WorkspaceID, ActorID: by.ID, Note: in.Note, RemindAt: in.RemindAt, CreatedAt: now}
	if in.MessageID != ids.Nil {
		m, err := s.Conversation.MessageByID(ctx, in.MessageID)
		if err != nil {
			return Item{}, err
		}
		ok, err := s.Conversation.CanRead(ctx, by, m.ChannelID)
		if err != nil {
			return Item{}, err
		}
		if !ok {
			return Item{}, db.ErrNotFound
		}
		it.MessageID, it.ChannelID = m.ID, m.ChannelID
	}
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		if it.MessageID != ids.Nil {
			old, err := s.Store.ByMessage(ctx, by.ID, it.MessageID)
			if err == nil {
				// Saved again: back in the list, with the new reminder or note.
				old.DoneAt = nil
				if in.RemindAt != nil {
					old.RemindAt, old.RemindedAt = in.RemindAt, nil
				}
				if in.Note != "" {
					old.Note = in.Note
				}
				if err := s.Store.Update(ctx, old); err != nil {
					return err
				}
				it = old
				seq, err = s.appendChanged(ctx, by, it, false)
				return err
			}
			if !errors.Is(err, db.ErrNotFound) {
				return err
			}
		}
		n, err := s.Store.Count(ctx, by.WorkspaceID, by.ID)
		if err != nil {
			return err
		}
		if n >= MaxItems {
			return ErrTooMany
		}
		if err := s.Store.Insert(ctx, it); err != nil {
			return err
		}
		seq, err = s.appendChanged(ctx, by, it, false)
		return err
	})
	if err != nil {
		return Item{}, err
	}
	s.Bus.Notify(seq)
	return it, nil
}

// UpdateInput changes an item: Done marks it done or not; SetRemind
// replaces the reminder (RemindAt nil clears it).
type UpdateInput struct {
	Done      *bool
	SetRemind bool
	RemindAt  *time.Time
}

// Update changes the caller's own item; anyone else's is not found.
func (s *Service) Update(ctx context.Context, by identity.Actor, id ids.ID, in UpdateInput) (Item, error) {
	if in.SetRemind {
		if err := s.checkRemindAt(in.RemindAt); err != nil {
			return Item{}, err
		}
	}
	var it Item
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		var err error
		if it, err = s.own(ctx, by, id); err != nil {
			return err
		}
		if in.Done != nil {
			if *in.Done {
				now := s.Clock.Now()
				it.DoneAt = &now
			} else {
				it.DoneAt = nil
			}
		}
		if in.SetRemind {
			if in.RemindAt == nil && it.MessageID == ids.Nil {
				return ErrEmpty // a reminder without a message is its reminder
			}
			it.RemindAt, it.RemindedAt = in.RemindAt, nil
		}
		if err := s.Store.Update(ctx, it); err != nil {
			return err
		}
		seq, err = s.appendChanged(ctx, by, it, false)
		return err
	})
	if err != nil {
		return Item{}, err
	}
	s.Bus.Notify(seq)
	return it, nil
}

// Delete removes the caller's own item.
func (s *Service) Delete(ctx context.Context, by identity.Actor, id ids.ID) error {
	var seq int64
	err := s.Tx.InTx(ctx, func(ctx context.Context) error {
		it, err := s.own(ctx, by, id)
		if err != nil {
			return err
		}
		if err := s.Store.Delete(ctx, id); err != nil {
			return err
		}
		seq, err = s.appendChanged(ctx, by, it, true)
		return err
	})
	if err != nil {
		return err
	}
	s.Bus.Notify(seq)
	return nil
}

func (s *Service) own(ctx context.Context, by identity.Actor, id ids.ID) (Item, error) {
	if by.Kind != identity.KindHuman {
		return Item{}, ErrForbidden
	}
	it, err := s.Store.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if it.ActorID != by.ID || it.WorkspaceID != by.WorkspaceID {
		return Item{}, db.ErrNotFound
	}
	return it, nil
}

// List returns a page of the caller's items, newest first, each with its
// message while the caller can still read it (a message they lost access
// to shows nothing of itself). next is the cursor of the following page.
func (s *Service) List(ctx context.Context, by identity.Actor, f Filter, after *Cursor) ([]View, *Cursor, error) {
	if by.Kind != identity.KindHuman {
		return nil, nil, ErrForbidden
	}
	if f == "" {
		f = FilterAll
	}
	if f != FilterAll && f != FilterReminders && f != FilterDone {
		return nil, nil, ErrBadFilter
	}
	items, err := s.Store.List(ctx, by.WorkspaceID, by.ID, f, after, PageSize+1)
	if err != nil {
		return nil, nil, err
	}
	var next *Cursor
	if len(items) > PageSize {
		items = items[:PageSize]
		last := items[PageSize-1]
		next = &Cursor{At: last.CreatedAt, ID: last.ID}
	}
	readable := map[ids.ID]bool{}
	out := make([]View, 0, len(items))
	for _, it := range items {
		v := View{Item: it}
		if it.MessageID != ids.Nil {
			ok, seen := readable[it.ChannelID]
			if !seen {
				ok, err = s.Conversation.CanRead(ctx, by, it.ChannelID)
				if err != nil {
					return nil, nil, err
				}
				readable[it.ChannelID] = ok
			}
			if ok {
				if m, err := s.Conversation.MessageByID(ctx, it.MessageID); err == nil {
					v.Message = &m
				}
				if c, err := s.Conversation.ChannelByID(ctx, it.ChannelID); err == nil {
					v.Channel = &c
				}
			}
		}
		out = append(out, v)
	}
	return out, next, nil
}

// MessageLink is the chip the PWA turns into a link to a message
// ([message:<channel>/<message>], with /<thread root> for a reply).
func MessageLink(m conversation.Message) string {
	if m.ThreadRootID != ids.Nil {
		return fmt.Sprintf("[message:%s/%s/%s]", m.ChannelID, m.ID, m.ThreadRootID)
	}
	return fmt.Sprintf("[message:%s/%s]", m.ChannelID, m.ID)
}

// reminderBody is the system message of a due reminder. It names the
// message only while the person can still read it.
func reminderBody(note string, m *conversation.Message) string {
	var b strings.Builder
	b.WriteString("⏰ Emlékeztető")
	if note != "" {
		b.WriteString(": ")
		b.WriteString(note)
	}
	if m != nil {
		b.WriteString("\n")
		b.WriteString(MessageLink(*m))
	}
	return b.String()
}

// RunDue posts every due reminder as a system message in its owner's own
// DM. Each one is claimed in the transaction that posts it, so it is sent
// once, also with several server instances. ctx is in system mode; each
// reminder is handled in its own workspace's scope.
func (s *Service) RunDue(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	due, err := s.Store.Due(ctx, now, 100)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, it := range due {
		wctx := db.WithWorkspace(ctx, it.WorkspaceID)
		var seq int64
		err := s.Tx.InTx(wctx, func(ctx context.Context) error {
			claimed, err := s.Store.ClaimReminder(ctx, it.ID, now)
			if err != nil || !claimed {
				return err
			}
			owner, err := s.Directory.ActorByID(ctx, it.ActorID)
			if err != nil {
				return err
			}
			var msg *conversation.Message
			if it.MessageID != ids.Nil {
				if ok, err := s.Conversation.CanRead(ctx, owner, it.ChannelID); err == nil && ok {
					if m, err := s.Conversation.MessageByID(ctx, it.MessageID); err == nil {
						msg = &m
					}
				}
			}
			dm, err := s.Conversation.SelfDM(ctx, owner)
			if err != nil {
				return err
			}
			posted, err := s.Conversation.PostSystemMessage(ctx, it.WorkspaceID, dm.ID, ids.Nil, owner.ID, reminderBody(it.Note, msg), nil)
			if err != nil {
				return err
			}
			p := Reminded{ItemID: it.ID.String(), ReminderMessage: posted.ID.String(), ReminderChannel: dm.ID.String()}
			if msg != nil {
				p.MessageID, p.ChannelID = msg.ID.String(), msg.ChannelID.String()
			}
			ev, err := eventlog.NewEvent(s.IDs.New(), it.WorkspaceID, EventReminded, owner.ID, ids.Nil, "saved_item", it.ID, p, now)
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
			if ctx.Err() != nil {
				return sent, err
			}
			continue // the next run tries again
		}
		if seq > 0 {
			sent++
			s.Bus.Notify(seq)
		}
	}
	return sent, nil
}

// Loop runs RunDue periodically until ctx ends.
func (s *Service) Loop(ctx context.Context, interval time.Duration, logf func(msg string, args ...any)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if n, err := s.RunDue(ctx); err != nil && ctx.Err() == nil {
			logf("reminders: run failed", "err", err)
		} else if n > 0 {
			logf("reminders: sent", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) appendChanged(ctx context.Context, by identity.Actor, it Item, deleted bool) (int64, error) {
	p := Changed{ItemID: it.ID.String(), Deleted: deleted}
	if it.MessageID != ids.Nil {
		p.MessageID = it.MessageID.String()
	}
	ev, err := eventlog.NewEvent(s.IDs.New(), by.WorkspaceID, EventChanged, by.ID, ids.Nil, "saved_item", it.ID, p, s.Clock.Now())
	if err != nil {
		return 0, err
	}
	if err := s.Events.Append(ctx, &ev); err != nil {
		return 0, err
	}
	return ev.Seq, nil
}
