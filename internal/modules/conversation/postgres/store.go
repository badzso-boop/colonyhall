// Package postgres implements conversation.Store on Postgres. SQL lives in
// queries.sql; sqlcgen is generated from it by sqlc.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/modules/conversation/postgres/sqlcgen"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements conversation.Store.
type Store struct{ DB *db.Postgres }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries { return sqlcgen.New(s.DB.Q(ctx)) }

// CreateChannel inserts a channel.
func (s *Store) CreateChannel(ctx context.Context, c conversation.Channel) error {
	err := s.q(ctx).CreateChannel(ctx, sqlcgen.CreateChannelParams{
		ID: c.ID, WorkspaceID: c.WorkspaceID, Kind: string(c.Kind), Name: c.Name,
		Description: c.Description, MsgCount: c.MsgCount, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt,
	})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// ChannelByID loads a channel.
func (s *Store) ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error) {
	return oneChannel(s.q(ctx).ChannelByID(ctx, id))
}

// ChannelByName loads a channel by name.
func (s *Store) ChannelByName(ctx context.Context, workspaceID ids.ID, name string) (conversation.Channel, error) {
	return oneChannel(s.q(ctx).ChannelByName(ctx, sqlcgen.ChannelByNameParams{WorkspaceID: workspaceID, Name: name}))
}

// ChannelsForActor lists channels with the actor's unread counts.
func (s *Store) ChannelsForActor(ctx context.Context, workspaceID, actorID ids.ID, all bool) ([]conversation.ChannelView, error) {
	rows, err := s.q(ctx).ChannelsForActor(ctx, sqlcgen.ChannelsForActorParams{
		ActorID: actorID, WorkspaceID: workspaceID, AllChannels: all,
	})
	if err != nil {
		return nil, err
	}
	out := make([]conversation.ChannelView, 0, len(rows))
	for _, r := range rows {
		var compactThreshold *int
		if r.CompactThresholdTokens != nil {
			v := int(*r.CompactThresholdTokens)
			compactThreshold = &v
		}
		out = append(out, conversation.ChannelView{
			Channel: conversation.Channel{ID: r.ID, WorkspaceID: r.WorkspaceID, Kind: conversation.ChannelKind(r.Kind),
				Name: r.Name, Description: r.Description, CompactThresholdTokens: compactThreshold, MsgCount: r.MsgCount, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt},
			IsMember: r.IsMember, Unread: r.Unread, Mentions: r.Mentions,
		})
	}
	return out, nil
}

// AddMember inserts a membership.
func (s *Store) AddMember(ctx context.Context, m conversation.Member) error {
	err := s.q(ctx).AddMember(ctx, sqlcgen.AddMemberParams{
		ChannelID: m.ChannelID, ActorID: m.ActorID, CanWrite: m.CanWrite,
		MsgCount: m.MsgCount, MentionCount: m.MentionCount, JoinedAt: m.JoinedAt,
	})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// RemoveMember soft-removes a membership.
func (s *Store) RemoveMember(ctx context.Context, channelID, actorID ids.ID, at time.Time) error {
	return s.q(ctx).RemoveMember(ctx, sqlcgen.RemoveMemberParams{
		ChannelID: channelID,
		ActorID:   actorID,
		LeftAt:    &at,
	})
}

// Member loads a membership.
func (s *Store) Member(ctx context.Context, channelID, actorID ids.ID) (conversation.Member, error) {
	r, err := s.q(ctx).Member(ctx, sqlcgen.MemberParams{ChannelID: channelID, ActorID: actorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return conversation.Member{}, db.ErrNotFound
	}
	if err != nil {
		return conversation.Member{}, err
	}
	return conversation.Member{ChannelID: r.ChannelID, ActorID: r.ActorID, CanWrite: r.CanWrite,
		MsgCount: r.MsgCount, MentionCount: r.MentionCount, JoinedAt: r.JoinedAt, LeftAt: r.LeftAt}, nil
}

// MemberIDs lists the channel's members.
func (s *Store) MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error) {
	return s.q(ctx).MemberIDs(ctx, channelID)
}

// ListMembers lists the channel's active members.
func (s *Store) ListMembers(ctx context.Context, channelID ids.ID) ([]conversation.Member, error) {
	rows, err := s.q(ctx).ListMembers(ctx, channelID)
	if err != nil {
		return nil, err
	}
	out := make([]conversation.Member, len(rows))
	for i, r := range rows {
		out[i] = conversation.Member{
			ChannelID:    r.ChannelID,
			ActorID:      r.ActorID,
			CanWrite:     r.CanWrite,
			MsgCount:     r.MsgCount,
			MentionCount: r.MentionCount,
			JoinedAt:     r.JoinedAt,
			LeftAt:       r.LeftAt,
		}
	}
	return out, nil
}

// InsertMessage inserts a message.
func (s *Store) InsertMessage(ctx context.Context, m conversation.Message) error {
	mentions := m.Mentions
	if mentions == nil {
		mentions = []ids.ID{} // a nil slice would be stored as NULL
	}
	err := s.q(ctx).InsertMessage(ctx, sqlcgen.InsertMessageParams{
		ID: m.ID, WorkspaceID: m.WorkspaceID, ChannelID: m.ChannelID, ThreadRootID: nullID(m.ThreadRootID),
		AuthorID: m.AuthorID, ClientMsgID: nullString(m.ClientMsgID), Kind: m.Kind, Body: m.Body,
		Mentions: mentions, ReplyCount: int32(m.ReplyCount), LastReplyAt: m.LastReplyAt, CreatedAt: m.CreatedAt,
		OriginChannelID: nullID(m.OriginChannelID), OriginThreadID: nullID(m.OriginThreadID),
	})
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// MessageByID loads a message.
func (s *Store) MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error) {
	return oneMessage(s.q(ctx).MessageByID(ctx, id))
}

// MessageByClientID loads a message by the author's client id.
func (s *Store) MessageByClientID(ctx context.Context, authorID ids.ID, clientMsgID string) (conversation.Message, error) {
	return oneMessage(s.q(ctx).MessageByClientID(ctx, sqlcgen.MessageByClientIDParams{AuthorID: authorID, ClientMsgID: &clientMsgID}))
}

// MessagesByIDs loads several messages, oldest first.
func (s *Store) MessagesByIDs(ctx context.Context, idList []ids.ID) ([]conversation.Message, error) {
	return manyMessages(s.q(ctx).MessagesByIDs(ctx, idList))
}

// ListMessages returns a page, oldest first.
func (s *Store) ListMessages(ctx context.Context, channelID, before ids.ID, limit int) ([]conversation.Message, error) {
	return manyMessages(s.q(ctx).ListMessages(ctx, sqlcgen.ListMessagesParams{
		ChannelID: channelID, Before: nullID(before), PageSize: int32(limit),
	}))
}

// IncrementMsgCount bumps the channel's message count.
func (s *Store) IncrementMsgCount(ctx context.Context, channelID ids.ID) (int64, error) {
	return s.q(ctx).IncrementMsgCount(ctx, channelID)
}

// SetSeen records what the member has seen.
func (s *Store) SetSeen(ctx context.Context, channelID, actorID ids.ID, count int64) error {
	return s.q(ctx).SetSeen(ctx, sqlcgen.SetSeenParams{Seen: count, ChannelID: channelID, ActorID: actorID})
}

// IncrementMentions bumps the mention counters.
func (s *Store) IncrementMentions(ctx context.Context, channelID ids.ID, actorIDs []ids.ID) error {
	if len(actorIDs) == 0 {
		return nil
	}
	return s.q(ctx).IncrementMentions(ctx, sqlcgen.IncrementMentionsParams{ChannelID: channelID, ActorIds: actorIDs})
}

// BumpThread updates the root's reply counters.
func (s *Store) BumpThread(ctx context.Context, rootID ids.ID, at time.Time) error {
	return s.q(ctx).BumpThread(ctx, sqlcgen.BumpThreadParams{At: at, ID: rootID})
}

func oneChannel(r sqlcgen.Channel, err error) (conversation.Channel, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return conversation.Channel{}, db.ErrNotFound
	}
	if err != nil {
		return conversation.Channel{}, err
	}
	var compactThreshold *int
	if r.CompactThresholdTokens != nil {
		v := int(*r.CompactThresholdTokens)
		compactThreshold = &v
	}
	return conversation.Channel{ID: r.ID, WorkspaceID: r.WorkspaceID, Kind: conversation.ChannelKind(r.Kind),
		Name: r.Name, Description: r.Description, CompactThresholdTokens: compactThreshold, MsgCount: r.MsgCount, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt}, nil
}

func toMessage(r sqlcgen.Message) conversation.Message {
	m := conversation.Message{ID: r.ID, WorkspaceID: r.WorkspaceID, ChannelID: r.ChannelID, AuthorID: r.AuthorID,
		Kind: r.Kind, Body: r.Body, Mentions: r.Mentions, ReplyCount: int(r.ReplyCount),
		LastReplyAt: r.LastReplyAt, CreatedAt: r.CreatedAt, DeletedAt: r.DeletedAt, DeletedBy: r.DeletedBy}
	if r.ThreadRootID != nil {
		m.ThreadRootID = *r.ThreadRootID
	}
	if r.ClientMsgID != nil {
		m.ClientMsgID = *r.ClientMsgID
	}
	if r.OriginChannelID != nil {
		m.OriginChannelID = *r.OriginChannelID
	}
	if r.OriginThreadID != nil {
		m.OriginThreadID = *r.OriginThreadID
	}
	return m
}

func oneMessage(r sqlcgen.Message, err error) (conversation.Message, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return conversation.Message{}, db.ErrNotFound
	}
	if err != nil {
		return conversation.Message{}, err
	}
	return toMessage(r), nil
}

func manyMessages(rows []sqlcgen.Message, err error) ([]conversation.Message, error) {
	if err != nil {
		return nil, err
	}
	out := make([]conversation.Message, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMessage(r))
	}
	return out, nil
}

func nullID(id ids.ID) *ids.ID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// AddReaction adds or restores a reaction.
func (s *Store) AddReaction(ctx context.Context, r conversation.Reaction) (bool, error) {
	n, err := s.q(ctx).AddReaction(ctx, sqlcgen.AddReactionParams{MessageID: r.MessageID, ActorID: r.ActorID, Emoji: r.Emoji, CreatedAt: r.CreatedAt})
	return n == 1, err
}

// RemoveReaction soft-removes a reaction.
func (s *Store) RemoveReaction(ctx context.Context, messageID, actorID ids.ID, emoji string, at time.Time) (bool, error) {
	n, err := s.q(ctx).RemoveReaction(ctx, sqlcgen.RemoveReactionParams{At: at, MessageID: messageID, ActorID: actorID, Emoji: emoji})
	return n == 1, err
}

// ReactionsFor returns active reactions on the messages.
func (s *Store) ReactionsFor(ctx context.Context, messageIDs []ids.ID) ([]conversation.Reaction, error) {
	rows, err := s.q(ctx).ReactionsFor(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	out := make([]conversation.Reaction, 0, len(rows))
	for _, r := range rows {
		out = append(out, conversation.Reaction{MessageID: r.MessageID, ActorID: r.ActorID, Emoji: r.Emoji, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// ThreadMessages returns the root and its replies.
func (s *Store) ThreadMessages(ctx context.Context, rootID ids.ID) ([]conversation.Message, error) {
	return manyMessages(s.q(ctx).ThreadMessages(ctx, rootID))
}

// MainStreamMessages returns top-level messages in the channel.
func (s *Store) MainStreamMessages(ctx context.Context, channelID ids.ID) ([]conversation.Message, error) {
	return manyMessages(s.q(ctx).MainStreamMessages(ctx, channelID))
}

// LastMessageNotBy returns the newest message by someone else.
func (s *Store) LastMessageNotBy(ctx context.Context, channelID, actorID ids.ID) (conversation.Message, error) {
	return oneMessage(s.q(ctx).LastMessageNotBy(ctx, sqlcgen.LastMessageNotByParams{ChannelID: channelID, AuthorID: actorID}))
}

// LatestTopLevelMessageByAuthor returns the newest top-level message in the channel by the author.
func (s *Store) LatestTopLevelMessageByAuthor(ctx context.Context, channelID, authorID ids.ID) (conversation.Message, error) {
	return oneMessage(s.q(ctx).LatestTopLevelMessageByAuthor(ctx, sqlcgen.LatestTopLevelMessageByAuthorParams{ChannelID: channelID, AuthorID: authorID}))
}

// RetractMessage marks a message as retracted.
func (s *Store) RetractMessage(ctx context.Context, id ids.ID, deletedBy ids.ID, at time.Time) error {
	_, err := s.q(ctx).RetractMessage(ctx, sqlcgen.RetractMessageParams{
		ID:        id,
		DeletedBy: uuid.UUID(deletedBy),
		DeletedAt: at,
	})
	return err
}
