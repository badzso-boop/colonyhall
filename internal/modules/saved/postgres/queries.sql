-- name: InsertSaved :exec
INSERT INTO saved_item (id, workspace_id, actor_id, message_id, channel_id, note, remind_at, reminded_at, done_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: UpdateSaved :exec
UPDATE saved_item SET note = $2, remind_at = $3, reminded_at = $4, done_at = $5 WHERE id = $1;

-- name: GetSaved :one
SELECT * FROM saved_item WHERE id = $1;

-- name: SavedByMessage :one
SELECT * FROM saved_item WHERE actor_id = $1 AND message_id = $2;

-- name: DeleteSaved :exec
DELETE FROM saved_item WHERE id = $1;

-- name: ListSaved :many
SELECT * FROM saved_item
WHERE workspace_id = sqlc.arg(workspace_id) AND actor_id = sqlc.arg(actor_id)
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (created_at, id) < (sqlc.narg(before_at)::timestamptz, sqlc.narg(before_id)::uuid))
  AND CASE sqlc.arg(filter)::text
        WHEN 'reminders' THEN remind_at IS NOT NULL AND done_at IS NULL
        WHEN 'done' THEN done_at IS NOT NULL
        ELSE true
      END
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_size);

-- name: CountSaved :one
SELECT count(*) FROM saved_item WHERE workspace_id = $1 AND actor_id = $2;

-- name: DueReminders :many
SELECT * FROM saved_item
WHERE remind_at <= sqlc.arg(now)::timestamptz AND reminded_at IS NULL AND done_at IS NULL
ORDER BY remind_at
LIMIT sqlc.arg(page_size);

-- name: ClaimReminder :execrows
UPDATE saved_item SET reminded_at = sqlc.arg(at)::timestamptz
WHERE id = sqlc.arg(id) AND reminded_at IS NULL AND done_at IS NULL AND remind_at <= sqlc.arg(at)::timestamptz;
