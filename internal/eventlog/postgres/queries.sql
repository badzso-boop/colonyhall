-- name: LockAppend :exec
-- Held until commit: makes event writes single-writer, so seq order = commit order.
SELECT pg_advisory_xact_lock(7243100001);

-- name: InsertEvent :one
INSERT INTO event (seq, id, workspace_id, type, actor_id, channel_id,
                   aggregate_type, aggregate_id, payload, schema_version, at)
VALUES (nextval('event_seq'), $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING seq;

-- name: EventsSince :many
SELECT * FROM event WHERE seq > $1 ORDER BY seq LIMIT $2;

-- name: Head :one
SELECT COALESCE(MAX(seq), 0)::bigint FROM event;

-- name: ConsumerOffset :one
SELECT last_seq FROM consumer_offset WHERE consumer_name = $1;

-- name: SetConsumerOffset :exec
INSERT INTO consumer_offset (consumer_name, last_seq, updated_at) VALUES ($1, $2, now())
ON CONFLICT (consumer_name) DO UPDATE SET last_seq = EXCLUDED.last_seq, updated_at = now();
