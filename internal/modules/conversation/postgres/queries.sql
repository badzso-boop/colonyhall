-- name: CreateChannel :exec
INSERT INTO channel (id, workspace_id, kind, name, description, msg_count, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ChannelByID :one
SELECT * FROM channel WHERE id = $1;

-- name: ChannelByName :one
SELECT * FROM channel WHERE workspace_id = $1 AND name = $2;

-- name: ChannelsForActor :many
SELECT c.id, c.workspace_id, c.kind, c.name, c.description, c.compact_threshold_tokens, c.msg_count, c.created_by, c.created_at,
       (m.actor_id IS NOT NULL AND m.left_at IS NULL)::boolean AS is_member,
       COALESCE(c.msg_count - m.msg_count, 0)::bigint AS unread,
       COALESCE(m.mention_count, 0)::bigint AS mentions
FROM channel c
LEFT JOIN channel_member m ON m.channel_id = c.id AND m.actor_id = sqlc.arg(actor_id) AND m.left_at IS NULL
WHERE c.workspace_id = sqlc.arg(workspace_id) AND (sqlc.arg(all_channels)::boolean OR (m.actor_id IS NOT NULL AND m.left_at IS NULL))
ORDER BY c.name;

-- name: AddMember :exec
INSERT INTO channel_member (channel_id, actor_id, can_write, msg_count, mention_count, joined_at, left_at)
VALUES ($1, $2, $3, $4, $5, $6, NULL)
ON CONFLICT (channel_id, actor_id)
DO UPDATE SET can_write = EXCLUDED.can_write,
              msg_count = EXCLUDED.msg_count,
              joined_at = EXCLUDED.joined_at,
              left_at = NULL;

-- name: Member :one
SELECT * FROM channel_member WHERE channel_id = $1 AND actor_id = $2 AND left_at IS NULL;

-- name: MemberIDs :many
SELECT actor_id FROM channel_member WHERE channel_id = $1 AND left_at IS NULL ORDER BY actor_id;

-- name: RemoveMember :exec
UPDATE channel_member
SET left_at = $3
WHERE channel_id = $1 AND actor_id = $2 AND left_at IS NULL;

-- name: ListMembers :many
SELECT channel_id, actor_id, can_write, msg_count, mention_count, joined_at, left_at
FROM channel_member
WHERE channel_id = $1 AND left_at IS NULL
ORDER BY joined_at ASC;

-- name: InsertMessage :exec
INSERT INTO message (id, workspace_id, channel_id, thread_root_id, author_id, client_msg_id, kind, body,
                     mentions, reply_count, last_reply_at, created_at, origin_channel_id, origin_thread_id,
                     answers_message_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: LatestTopLevelMessageByAuthor :one
SELECT * FROM message
WHERE channel_id = $1 AND author_id = $2 AND thread_root_id IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: MessageByID :one
SELECT * FROM message WHERE id = $1;

-- name: MessageByClientID :one
SELECT * FROM message WHERE author_id = $1 AND client_msg_id = $2;

-- name: MessagesByIDs :many
SELECT * FROM message WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY id;

-- name: ListMessages :many
SELECT * FROM (
    SELECT * FROM message
    WHERE channel_id = sqlc.arg(channel_id) AND deleted_at IS NULL
      AND (sqlc.narg(before)::uuid IS NULL OR id < sqlc.narg(before)::uuid)
    ORDER BY id DESC
    LIMIT sqlc.arg(page_size)
) page ORDER BY id;

-- name: IncrementMsgCount :one
UPDATE channel SET msg_count = msg_count + 1 WHERE id = $1 RETURNING msg_count;

-- name: SetSeen :exec
UPDATE channel_member SET msg_count = GREATEST(msg_count, sqlc.arg(seen)::bigint), mention_count = 0
WHERE channel_id = sqlc.arg(channel_id) AND actor_id = sqlc.arg(actor_id) AND left_at IS NULL;

-- name: IncrementMentions :exec
UPDATE channel_member SET mention_count = mention_count + 1
WHERE channel_id = sqlc.arg(channel_id) AND actor_id = ANY(sqlc.arg(actor_ids)::uuid[]) AND left_at IS NULL;

-- name: BumpThread :exec
UPDATE message SET reply_count = reply_count + 1, last_reply_at = sqlc.arg(at)::timestamptz WHERE id = sqlc.arg(id);

-- name: AddReaction :execrows
INSERT INTO reaction (message_id, actor_id, emoji, created_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (message_id, actor_id, emoji) DO UPDATE SET removed_at = NULL, created_at = EXCLUDED.created_at
WHERE reaction.removed_at IS NOT NULL;

-- name: RemoveReaction :execrows
UPDATE reaction SET removed_at = sqlc.arg(at)::timestamptz
WHERE message_id = sqlc.arg(message_id) AND actor_id = sqlc.arg(actor_id) AND emoji = sqlc.arg(emoji) AND removed_at IS NULL;

-- name: ReplyParticipants :many
-- Up to max_per_root distinct reply authors per thread, most recent first.
SELECT t.root_id::uuid AS root_id, t.author_id FROM (
  SELECT thread_root_id AS root_id, author_id,
         -- uuid has no max(); the reply time orders, the id breaks ties.
         row_number() OVER (PARTITION BY thread_root_id ORDER BY max(created_at) DESC, max(id::text) DESC) AS rn
  FROM message
  WHERE thread_root_id = ANY(sqlc.arg(root_ids)::uuid[]) AND deleted_at IS NULL
  GROUP BY thread_root_id, author_id
) t
WHERE t.rn <= sqlc.arg(max_per_root)::int
ORDER BY t.root_id, t.rn;

-- name: ReactionsFor :many
SELECT message_id, actor_id, emoji, created_at FROM reaction
WHERE message_id = ANY(sqlc.arg(message_ids)::uuid[]) AND removed_at IS NULL
ORDER BY created_at, actor_id;

-- name: ThreadMessages :many
SELECT * FROM message WHERE (id = sqlc.arg(root_id) OR thread_root_id = sqlc.arg(root_id)) AND deleted_at IS NULL
ORDER BY id;

-- name: MainStreamMessages :many
SELECT * FROM message WHERE channel_id = $1 AND thread_root_id IS NULL AND deleted_at IS NULL
ORDER BY id;

-- name: LastMessageNotBy :one
SELECT * FROM message WHERE channel_id = $1 AND author_id <> $2 AND deleted_at IS NULL ORDER BY id DESC LIMIT 1;

-- name: RetractMessage :execrows
UPDATE message
SET deleted_at = sqlc.arg(deleted_at)::timestamptz,
    deleted_by = sqlc.arg(deleted_by)::uuid
WHERE id = sqlc.arg(id) AND deleted_at IS NULL;

