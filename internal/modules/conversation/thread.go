package conversation

import (
	"context"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Thread returns a thread root and its replies (oldest first). Asking with
// a reply's ID returns that reply's whole thread.
func (s *Service) Thread(ctx context.Context, by identity.Actor, messageID ids.ID) (Message, []Message, error) {
	m, err := s.Store.MessageByID(ctx, messageID)
	if err != nil {
		return Message{}, nil, err
	}
	ok, err := s.CanRead(ctx, by, m.ChannelID)
	if err != nil {
		return Message{}, nil, err
	}
	if !ok {
		return Message{}, nil, ErrForbidden
	}
	rootID := m.ID
	if m.ThreadRootID != ids.Nil {
		rootID = m.ThreadRootID
	}
	all, err := s.Store.ThreadMessages(ctx, rootID)
	if err != nil {
		return Message{}, nil, err
	}
	all, err = s.withReplyParticipants(ctx, all)
	if err != nil {
		return Message{}, nil, err
	}
	all, err = s.withReactions(ctx, all)
	if err != nil {
		return Message{}, nil, err
	}
	all, err = s.withAttachments(ctx, all)
	if err != nil {
		return Message{}, nil, err
	}
	s.enrichOrigins(ctx, by, all)
	all = maskRetracted(all)
	var root Message
	var replies []Message
	for _, x := range all {
		if x.ID == rootID {
			root = x
		} else {
			replies = append(replies, x)
		}
	}
	return root, replies, nil
}

// Thread list limits.
const (
	DefaultThreadPage = 20
	MaxThreadPage     = 50
	// LatestRepliesShown: replies shown on a "Szálaim" card.
	LatestRepliesShown = 2
	// maxUnreadThreadsCounted bounds the sidebar's unread thread count.
	maxUnreadThreadsCounted = 500
)

// ThreadPage is one page of the viewer's threads.
type ThreadPage struct {
	Threads []ThreadSummary
	Next    *ThreadCursor
	// UnreadThreads counts the viewer's threads with unread replies; threads
	// in channels the viewer muted count only when the viewer is mentioned.
	UnreadThreads int
}

// MyThreads lists the threads the caller (a human) wrote, replied in or was
// mentioned in, newest reply first, in channels the caller is still a member of.
func (s *Service) MyThreads(ctx context.Context, by identity.Actor, after *ThreadCursor, limit int, unreadOnly bool) (ThreadPage, error) {
	if by.Kind != identity.KindHuman {
		return ThreadPage{}, ErrForbidden
	}
	if limit <= 0 {
		limit = DefaultThreadPage
	}
	limit = min(limit, MaxThreadPage)
	list, err := s.Store.ThreadsForActor(ctx, by.WorkspaceID, by.ID, after, limit+1, unreadOnly)
	if err != nil {
		return ThreadPage{}, err
	}
	var page ThreadPage
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1].Root
		page.Next = &ThreadCursor{LastReplyAt: *last.LastReplyAt, RootID: last.ID}
	}
	roots := make([]Message, len(list))
	for i, t := range list {
		roots[i] = t.Root
	}
	if roots, err = s.withReplyParticipants(ctx, roots); err != nil {
		return ThreadPage{}, err
	}
	if roots, err = s.withReactions(ctx, roots); err != nil {
		return ThreadPage{}, err
	}
	if roots, err = s.withAttachments(ctx, roots); err != nil {
		return ThreadPage{}, err
	}
	s.enrichOrigins(ctx, by, roots)
	channels := map[ids.ID]Channel{}
	for i := range list {
		list[i].Root = roots[i]
		c, ok := channels[roots[i].ChannelID]
		if !ok {
			if c, err = s.Store.ChannelByID(ctx, roots[i].ChannelID); err != nil {
				return ThreadPage{}, err
			}
			channels[c.ID] = c
		}
		list[i].Channel = c
		all, err := s.Store.ThreadMessages(ctx, roots[i].ID)
		if err != nil {
			return ThreadPage{}, err
		}
		var replies []Message
		for _, m := range all {
			if m.ThreadRootID != ids.Nil && m.DeletedAt == nil {
				replies = append(replies, m)
			}
		}
		if len(replies) > LatestRepliesShown {
			replies = replies[len(replies)-LatestRepliesShown:]
		}
		if replies, err = s.withAttachments(ctx, replies); err != nil {
			return ThreadPage{}, err
		}
		list[i].LatestReplies = replies
	}
	page.Threads = list
	page.UnreadThreads, err = s.unreadThreads(ctx, by)
	return page, err
}

func (s *Service) unreadThreads(ctx context.Context, by identity.Actor) (int, error) {
	unread, err := s.Store.ThreadsForActor(ctx, by.WorkspaceID, by.ID, nil, maxUnreadThreadsCounted, true)
	if err != nil || len(unread) == 0 {
		return 0, err
	}
	mutes, err := s.Store.Mutes(ctx, by.ID)
	if err != nil {
		return 0, err
	}
	now := s.Clock.Now()
	muted := map[ids.ID]bool{}
	for _, m := range mutes {
		if m.Active(now) {
			muted[m.ChannelID] = true
		}
	}
	n := 0
	for _, t := range unread {
		if !muted[t.Root.ChannelID] || t.Mentioned {
			n++
		}
	}
	return n, nil
}

// MarkThreadRead records that the caller has seen the thread to its end.
func (s *Service) MarkThreadRead(ctx context.Context, by identity.Actor, rootID ids.ID) error {
	if by.Kind != identity.KindHuman {
		return ErrForbidden
	}
	root, err := s.Store.MessageByID(ctx, rootID)
	if err != nil {
		return err
	}
	if root.ThreadRootID != ids.Nil {
		return ErrBadThread
	}
	ok, err := s.CanRead(ctx, by, root.ChannelID)
	if err != nil {
		return err
	}
	if !ok {
		return db.ErrNotFound
	}
	return s.Store.SetThreadRead(ctx, by.ID, rootID, s.Clock.Now())
}

// MarkAllThreadsRead marks every thread of the caller with unread replies as read.
func (s *Service) MarkAllThreadsRead(ctx context.Context, by identity.Actor) (int, error) {
	if by.Kind != identity.KindHuman {
		return 0, ErrForbidden
	}
	unread, err := s.Store.ThreadsForActor(ctx, by.WorkspaceID, by.ID, nil, maxUnreadThreadsCounted, true)
	if err != nil {
		return 0, err
	}
	now := s.Clock.Now()
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		for _, t := range unread {
			if err := s.Store.SetThreadRead(ctx, by.ID, t.Root.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
	return len(unread), err
}
