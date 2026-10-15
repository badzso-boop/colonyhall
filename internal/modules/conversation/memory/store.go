// Package memory is the in-memory reference implementation of conversation.Store.
package memory

import (
	"bytes"
	"context"
	"sort"
	"sync"
	"time"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type memberKey struct{ channel, actor ids.ID }

// Store implements conversation.Store in memory.
type Store struct {
	mu       sync.RWMutex
	channels map[ids.ID]conversation.Channel
	members  map[memberKey]conversation.Member
	messages map[ids.ID]conversation.Message
	reacts   map[reactKey]reactRow
}

type reactKey struct {
	message, actor ids.ID
	emoji          string
}

type reactRow struct {
	r       conversation.Reaction
	removed bool
}

// New returns an empty store.
func New() *Store {
	return &Store{
		channels: map[ids.ID]conversation.Channel{},
		members:  map[memberKey]conversation.Member{},
		messages: map[ids.ID]conversation.Message{},
		reacts:   map[reactKey]reactRow{},
	}
}

// CreateChannel stores a channel, rejecting duplicate names.
func (s *Store) CreateChannel(_ context.Context, c conversation.Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.channels {
		if o.WorkspaceID == c.WorkspaceID && o.Name == c.Name {
			return db.ErrConflict
		}
	}
	s.channels[c.ID] = c
	return nil
}

// ChannelByID loads a channel.
func (s *Store) ChannelByID(_ context.Context, id ids.ID) (conversation.Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.channels[id]
	if !ok {
		return c, db.ErrNotFound
	}
	return c, nil
}

// ChannelByName loads a channel by name.
func (s *Store) ChannelByName(_ context.Context, workspaceID ids.ID, name string) (conversation.Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.channels {
		if c.WorkspaceID == workspaceID && c.Name == name {
			return c, nil
		}
	}
	return conversation.Channel{}, db.ErrNotFound
}

// ChannelsForActor lists channels with unread counts, sorted by name.
func (s *Store) ChannelsForActor(_ context.Context, workspaceID, actorID ids.ID, all bool) ([]conversation.ChannelView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []conversation.ChannelView
	for _, c := range s.channels {
		if c.WorkspaceID != workspaceID {
			continue
		}
		m, isMember := s.members[memberKey{c.ID, actorID}]
		if !isMember && !all {
			continue
		}
		v := conversation.ChannelView{Channel: c, IsMember: isMember}
		if isMember {
			v.Unread = c.MsgCount - m.MsgCount
			v.Mentions = m.MentionCount
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// AddMember stores a membership.
func (s *Store) AddMember(_ context.Context, m conversation.Member) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := memberKey{m.ChannelID, m.ActorID}
	if _, ok := s.members[k]; ok {
		return db.ErrConflict
	}
	s.members[k] = m
	return nil
}

// Member loads a membership.
func (s *Store) Member(_ context.Context, channelID, actorID ids.ID) (conversation.Member, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.members[memberKey{channelID, actorID}]
	if !ok {
		return m, db.ErrNotFound
	}
	return m, nil
}

// MemberIDs lists the channel's members.
func (s *Store) MemberIDs(_ context.Context, channelID ids.ID) ([]ids.ID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ids.ID
	for k := range s.members {
		if k.channel == channelID {
			out = append(out, k.actor)
		}
	}
	sortIDs(out)
	return out, nil
}

// InsertMessage stores a message, rejecting a duplicate client id.
func (s *Store) InsertMessage(_ context.Context, m conversation.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.ClientMsgID != "" {
		for _, o := range s.messages {
			if o.AuthorID == m.AuthorID && o.ClientMsgID == m.ClientMsgID {
				return db.ErrConflict
			}
		}
	}
	m.Mentions = append([]ids.ID(nil), m.Mentions...)
	s.messages[m.ID] = m
	return nil
}

// MessageByID loads a message.
func (s *Store) MessageByID(_ context.Context, id ids.ID) (conversation.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.messages[id]
	if !ok {
		return m, db.ErrNotFound
	}
	return m, nil
}

// MessageByClientID loads a message by the author's client id.
func (s *Store) MessageByClientID(_ context.Context, authorID ids.ID, clientMsgID string) (conversation.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.messages {
		if m.AuthorID == authorID && m.ClientMsgID == clientMsgID {
			return m, nil
		}
	}
	return conversation.Message{}, db.ErrNotFound
}

// MessagesByIDs loads several messages, oldest first.
func (s *Store) MessagesByIDs(_ context.Context, idList []ids.ID) ([]conversation.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []conversation.Message
	for _, id := range idList {
		if m, ok := s.messages[id]; ok {
			out = append(out, m)
		}
	}
	sortMessages(out)
	return out, nil
}

// ListMessages returns a page, oldest first.
func (s *Store) ListMessages(_ context.Context, channelID, before ids.ID, limit int) ([]conversation.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []conversation.Message
	for _, m := range s.messages {
		if m.ChannelID != channelID {
			continue
		}
		if before != ids.Nil && bytes.Compare(m.ID[:], before[:]) >= 0 {
			continue
		}
		all = append(all, m)
	}
	sortMessages(all)
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// IncrementMsgCount bumps the channel's message count.
func (s *Store) IncrementMsgCount(_ context.Context, channelID ids.ID) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.channels[channelID]
	if !ok {
		return 0, db.ErrNotFound
	}
	c.MsgCount++
	s.channels[channelID] = c
	return c.MsgCount, nil
}

// SetSeen records what the member has seen.
func (s *Store) SetSeen(_ context.Context, channelID, actorID ids.ID, count int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := memberKey{channelID, actorID}
	m, ok := s.members[k]
	if !ok {
		return nil
	}
	if count > m.MsgCount {
		m.MsgCount = count
	}
	m.MentionCount = 0
	s.members[k] = m
	return nil
}

// IncrementMentions bumps the mention counters.
func (s *Store) IncrementMentions(_ context.Context, channelID ids.ID, actorIDs []ids.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range actorIDs {
		k := memberKey{channelID, a}
		if m, ok := s.members[k]; ok {
			m.MentionCount++
			s.members[k] = m
		}
	}
	return nil
}

// BumpThread updates the root's reply counters.
func (s *Store) BumpThread(_ context.Context, rootID ids.ID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.messages[rootID]
	if !ok {
		return db.ErrNotFound
	}
	m.ReplyCount++
	m.LastReplyAt = &at
	s.messages[rootID] = m
	return nil
}

func sortMessages(ms []conversation.Message) {
	sort.Slice(ms, func(i, j int) bool { return bytes.Compare(ms[i].ID[:], ms[j].ID[:]) < 0 })
}

func sortIDs(list []ids.ID) {
	sort.Slice(list, func(i, j int) bool { return bytes.Compare(list[i][:], list[j][:]) < 0 })
}

// AddReaction adds or restores a reaction.
func (s *Store) AddReaction(_ context.Context, r conversation.Reaction) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := reactKey{r.MessageID, r.ActorID, r.Emoji}
	if row, ok := s.reacts[k]; ok && !row.removed {
		return false, nil
	}
	s.reacts[k] = reactRow{r: r}
	return true, nil
}

// RemoveReaction soft-removes a reaction.
func (s *Store) RemoveReaction(_ context.Context, messageID, actorID ids.ID, emoji string, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := reactKey{messageID, actorID, emoji}
	row, ok := s.reacts[k]
	if !ok || row.removed {
		return false, nil
	}
	row.removed = true
	s.reacts[k] = row
	return true, nil
}

// ReactionsFor returns active reactions on the messages, oldest first.
func (s *Store) ReactionsFor(_ context.Context, messageIDs []ids.ID) ([]conversation.Reaction, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	want := map[ids.ID]bool{}
	for _, id := range messageIDs {
		want[id] = true
	}
	var out []conversation.Reaction
	for _, row := range s.reacts {
		if !row.removed && want[row.r.MessageID] {
			out = append(out, row.r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return bytes.Compare(out[i].ActorID[:], out[j].ActorID[:]) < 0
	})
	return out, nil
}

// ThreadMessages returns the root and its replies, oldest first.
func (s *Store) ThreadMessages(_ context.Context, rootID ids.ID) ([]conversation.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []conversation.Message
	for _, m := range s.messages {
		if m.ID == rootID || m.ThreadRootID == rootID {
			out = append(out, m)
		}
	}
	sortMessages(out)
	return out, nil
}

// LastMessageNotBy returns the newest message by someone else.
func (s *Store) LastMessageNotBy(_ context.Context, channelID, actorID ids.ID) (conversation.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best *conversation.Message
	for _, m := range s.messages {
		if m.ChannelID != channelID || m.AuthorID == actorID {
			continue
		}
		if best == nil || bytes.Compare(m.ID[:], best.ID[:]) > 0 {
			m := m
			best = &m
		}
	}
	if best == nil {
		return conversation.Message{}, db.ErrNotFound
	}
	return *best, nil
}
