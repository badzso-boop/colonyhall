-- name: InsertDelivery :execrows
INSERT INTO delivery (id, workspace_id, recipient_id, item_kind, item_id, channel_id, flags, event_seq, created_at, from_actor_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (recipient_id, item_kind, item_id) DO NOTHING;

-- name: CountPending :one
SELECT count(*) FROM delivery WHERE recipient_id = $1 AND acked_at IS NULL;

-- name: Pending :many
SELECT * FROM delivery WHERE recipient_id = $1 AND acked_at IS NULL ORDER BY id LIMIT $2;

-- name: Ack :execrows
UPDATE delivery SET acked_at = sqlc.arg(at)::timestamptz
WHERE recipient_id = sqlc.arg(recipient_id) AND id = ANY(sqlc.arg(ids)::uuid[]) AND acked_at IS NULL;
