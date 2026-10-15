// Package postgres implements conversation.Store on Postgres.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Store implements conversation.Store.
type Store struct{ DB *db.Postgres }

const channelColumns = `id, workspace_id, kind, name, msg_count, created_by, created_at`
const messageColumns = `id, workspace_id, channel_id, thread_root_id, author_id, client_msg_id, kind, body,
	mentions, reply_count, last_reply_at, created_at`

// CreateChannel inserts a channel.
func (s *Store) CreateChannel(ctx context.Context, c conversation.Channel) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `INSERT INTO channel (`+channelColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.WorkspaceID, c.Kind, c.Name, c.MsgCount, c.CreatedBy, c.CreatedAt)
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// ChannelByID loads a channel.
func (s *Store) ChannelByID(ctx context.Context, id ids.ID) (conversation.Channel, error) {
	return scanChannel(s.DB.Q(ctx).QueryRow(ctx, `SELECT `+channelColumns+` FROM channel WHERE id = $1`, id))
}

// ChannelByName loads a channel by name.
func (s *Store) ChannelByName(ctx context.Context, workspaceID ids.ID, name string) (conversation.Channel, error) {
	return scanChannel(s.DB.Q(ctx).QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channel WHERE workspace_id = $1 AND name = $2`, workspaceID, name))
}

// ChannelsForActor lists channels with the actor's unread counts.
func (s *Store) ChannelsForActor(ctx context.Context, workspaceID, actorID ids.ID, all bool) ([]conversation.ChannelView, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `
		SELECT c.id, c.workspace_id, c.kind, c.name, c.msg_count, c.created_by, c.created_at,
		       m.actor_id IS NOT NULL, COALESCE(c.msg_count - m.msg_count, 0), COALESCE(m.mention_count, 0)
		FROM channel c
		LEFT JOIN channel_member m ON m.channel_id = c.id AND m.actor_id = $2
		WHERE c.workspace_id = $1 AND ($3 OR m.actor_id IS NOT NULL)
		ORDER BY c.name`, workspaceID, actorID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []conversation.ChannelView
	for rows.Next() {
		var v conversation.ChannelView
		if err := rows.Scan(&v.ID, &v.WorkspaceID, &v.Kind, &v.Name, &v.MsgCount, &v.CreatedBy, &v.CreatedAt,
			&v.IsMember, &v.Unread, &v.Mentions); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// AddMember inserts a membership.
func (s *Store) AddMember(ctx context.Context, m conversation.Member) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `
		INSERT INTO channel_member (channel_id, actor_id, can_write, msg_count, mention_count, joined_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, m.ChannelID, m.ActorID, m.CanWrite, m.MsgCount, m.MentionCount, m.JoinedAt)
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// Member loads a membership.
func (s *Store) Member(ctx context.Context, channelID, actorID ids.ID) (conversation.Member, error) {
	var m conversation.Member
	err := s.DB.Q(ctx).QueryRow(ctx, `
		SELECT channel_id, actor_id, can_write, msg_count, mention_count, joined_at
		FROM channel_member WHERE channel_id = $1 AND actor_id = $2`, channelID, actorID).
		Scan(&m.ChannelID, &m.ActorID, &m.CanWrite, &m.MsgCount, &m.MentionCount, &m.JoinedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, db.ErrNotFound
	}
	return m, err
}

// MemberIDs lists the channel's members.
func (s *Store) MemberIDs(ctx context.Context, channelID ids.ID) ([]ids.ID, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `SELECT actor_id FROM channel_member WHERE channel_id = $1 ORDER BY actor_id`, channelID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.ID])
}

// InsertMessage inserts a message.
func (s *Store) InsertMessage(ctx context.Context, m conversation.Message) error {
	if m.Mentions == nil {
		m.Mentions = []ids.ID{} // a nil slice would be stored as NULL
	}
	_, err := s.DB.Q(ctx).Exec(ctx, `INSERT INTO message (`+messageColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		m.ID, m.WorkspaceID, m.ChannelID, nullID(m.ThreadRootID), m.AuthorID, nullString(m.ClientMsgID),
		m.Kind, m.Body, m.Mentions, m.ReplyCount, m.LastReplyAt, m.CreatedAt)
	if db.IsUniqueViolation(err) {
		return db.ErrConflict
	}
	return err
}

// MessageByID loads a message.
func (s *Store) MessageByID(ctx context.Context, id ids.ID) (conversation.Message, error) {
	return scanMessage(s.DB.Q(ctx).QueryRow(ctx, `SELECT `+messageColumns+` FROM message WHERE id = $1`, id))
}

// MessageByClientID loads a message by the author's client id.
func (s *Store) MessageByClientID(ctx context.Context, authorID ids.ID, clientMsgID string) (conversation.Message, error) {
	return scanMessage(s.DB.Q(ctx).QueryRow(ctx,
		`SELECT `+messageColumns+` FROM message WHERE author_id = $1 AND client_msg_id = $2`, authorID, clientMsgID))
}

// MessagesByIDs loads several messages, oldest first.
func (s *Store) MessagesByIDs(ctx context.Context, idList []ids.ID) ([]conversation.Message, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `SELECT `+messageColumns+` FROM message WHERE id = ANY($1) ORDER BY id`, idList)
	if err != nil {
		return nil, err
	}
	return collectMessages(rows)
}

// ListMessages returns a page, oldest first.
func (s *Store) ListMessages(ctx context.Context, channelID, before ids.ID, limit int) ([]conversation.Message, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `
		SELECT * FROM (
			SELECT `+messageColumns+` FROM message
			WHERE channel_id = $1 AND deleted_at IS NULL AND ($2::uuid IS NULL OR id < $2)
			ORDER BY id DESC LIMIT $3
		) page ORDER BY id`, channelID, nullID(before), limit)
	if err != nil {
		return nil, err
	}
	return collectMessages(rows)
}

// IncrementMsgCount bumps the channel's message count.
func (s *Store) IncrementMsgCount(ctx context.Context, channelID ids.ID) (int64, error) {
	var n int64
	err := s.DB.Q(ctx).QueryRow(ctx,
		`UPDATE channel SET msg_count = msg_count + 1 WHERE id = $1 RETURNING msg_count`, channelID).Scan(&n)
	return n, err
}

// SetSeen records what the member has seen.
func (s *Store) SetSeen(ctx context.Context, channelID, actorID ids.ID, count int64) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `
		UPDATE channel_member SET msg_count = GREATEST(msg_count, $3), mention_count = 0
		WHERE channel_id = $1 AND actor_id = $2`, channelID, actorID, count)
	return err
}

// IncrementMentions bumps the mention counters.
func (s *Store) IncrementMentions(ctx context.Context, channelID ids.ID, actorIDs []ids.ID) error {
	if len(actorIDs) == 0 {
		return nil
	}
	_, err := s.DB.Q(ctx).Exec(ctx, `
		UPDATE channel_member SET mention_count = mention_count + 1
		WHERE channel_id = $1 AND actor_id = ANY($2)`, channelID, actorIDs)
	return err
}

// BumpThread updates the root's reply counters.
func (s *Store) BumpThread(ctx context.Context, rootID ids.ID, at time.Time) error {
	_, err := s.DB.Q(ctx).Exec(ctx,
		`UPDATE message SET reply_count = reply_count + 1, last_reply_at = $2 WHERE id = $1`, rootID, at)
	return err
}

func scanChannel(row pgx.Row) (conversation.Channel, error) {
	var c conversation.Channel
	err := row.Scan(&c.ID, &c.WorkspaceID, &c.Kind, &c.Name, &c.MsgCount, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, db.ErrNotFound
	}
	return c, err
}

func scanMessage(row pgx.Row) (conversation.Message, error) {
	var m conversation.Message
	var root *ids.ID
	var clientID *string
	err := row.Scan(&m.ID, &m.WorkspaceID, &m.ChannelID, &root, &m.AuthorID, &clientID, &m.Kind, &m.Body,
		&m.Mentions, &m.ReplyCount, &m.LastReplyAt, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, db.ErrNotFound
	}
	if root != nil {
		m.ThreadRootID = *root
	}
	if clientID != nil {
		m.ClientMsgID = *clientID
	}
	return m, err
}

func collectMessages(rows pgx.Rows) ([]conversation.Message, error) {
	defer rows.Close()
	var out []conversation.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
