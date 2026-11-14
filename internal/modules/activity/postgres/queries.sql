-- name: InsertActivity :execrows
INSERT INTO activity_item (id, workspace_id, recipient_id, kind, channel_id, message_id, thread_root_id, actor_id, ref_id, emoji, source_seq, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT ON CONSTRAINT activity_item_source_key DO NOTHING;

-- name: ListActivity :many
SELECT * FROM activity_item
WHERE workspace_id = sqlc.arg(workspace_id) AND recipient_id = sqlc.arg(recipient_id)
  AND (sqlc.arg(kind)::text = '' OR kind = sqlc.arg(kind)::text)
  AND (NOT sqlc.arg(unread_only)::boolean OR read_at IS NULL)
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: CountUnreadActivity :one
SELECT count(*) FROM activity_item WHERE workspace_id = $1 AND recipient_id = $2 AND read_at IS NULL;

-- name: MarkActivityRead :execrows
UPDATE activity_item SET read_at = sqlc.arg(at)::timestamptz
WHERE recipient_id = sqlc.arg(recipient_id) AND read_at IS NULL
  AND (cardinality(sqlc.arg(item_ids)::uuid[]) = 0 OR id = ANY(sqlc.arg(item_ids)::uuid[]))
  AND (sqlc.narg(channel_id)::uuid IS NULL OR channel_id = sqlc.narg(channel_id)::uuid);
