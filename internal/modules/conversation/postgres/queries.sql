-- name: CreateChannel :exec
INSERT INTO channel (id, workspace_id, kind, name, msg_count, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ChannelByID :one
SELECT * FROM channel WHERE id = $1;

-- name: ChannelByName :one
SELECT * FROM channel WHERE workspace_id = $1 AND name = $2;

-- name: ChannelsForActor :many
SELECT c.id, c.workspace_id, c.kind, c.name, c.msg_count, c.created_by, c.created_at,
       (m.actor_id IS NOT NULL)::boolean AS is_member,
       COALESCE(c.msg_count - m.msg_count, 0)::bigint AS unread,
       COALESCE(m.mention_count, 0)::bigint AS mentions
FROM channel c
LEFT JOIN channel_member m ON m.channel_id = c.id AND m.actor_id = sqlc.arg(actor_id)
WHERE c.workspace_id = sqlc.arg(workspace_id) AND (sqlc.arg(all_channels)::boolean OR m.actor_id IS NOT NULL)
ORDER BY c.name;

-- name: AddMember :exec
INSERT INTO channel_member (channel_id, actor_id, can_write, msg_count, mention_count, joined_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: Member :one
SELECT * FROM channel_member WHERE channel_id = $1 AND actor_id = $2;

-- name: MemberIDs :many
SELECT actor_id FROM channel_member WHERE channel_id = $1 ORDER BY actor_id;

-- name: InsertMessage :exec
INSERT INTO message (id, workspace_id, channel_id, thread_root_id, author_id, client_msg_id, kind, body,
                     mentions, reply_count, last_reply_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

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
WHERE channel_id = sqlc.arg(channel_id) AND actor_id = sqlc.arg(actor_id);

-- name: IncrementMentions :exec
UPDATE channel_member SET mention_count = mention_count + 1
WHERE channel_id = sqlc.arg(channel_id) AND actor_id = ANY(sqlc.arg(actor_ids)::uuid[]);

-- name: BumpThread :exec
UPDATE message SET reply_count = reply_count + 1, last_reply_at = sqlc.arg(at)::timestamptz WHERE id = sqlc.arg(id);
