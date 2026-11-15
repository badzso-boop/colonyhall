-- name: CreateSchedule :exec
INSERT INTO schedule (id, workspace_id, created_by, channel_id, thread_root_id, body, cron, timezone, catch_up, state, next_run_at, created_at)
VALUES ($1, $2, $3, $4, sqlc.narg(thread_root_id), $5, $6, $7, $8, $9, $10, $11);

-- name: ScheduleByID :one
SELECT * FROM schedule WHERE id = $1;

-- name: ListSchedules :many
SELECT * FROM schedule WHERE workspace_id = $1 ORDER BY next_run_at, id;

-- name: DueSchedules :many
-- Round 51: a suspended workspace's schedules wait (skip lists them).
SELECT * FROM schedule WHERE state = 'active' AND next_run_at <= sqlc.arg(now)
    AND NOT (workspace_id = ANY(sqlc.arg(skip)::uuid[]))
ORDER BY next_run_at, id LIMIT sqlc.arg(lim);

-- name: ClaimSchedule :execrows
UPDATE schedule SET next_run_at = sqlc.arg(next), state = sqlc.arg(state), last_run_at = sqlc.arg(at), run_count = run_count + 1
WHERE id = sqlc.arg(id) AND state = 'active' AND next_run_at = sqlc.arg(due_at);

-- name: RecordRun :exec
UPDATE schedule SET last_message_id = sqlc.narg(message_id), last_error = sqlc.arg(run_error) WHERE id = sqlc.arg(id);

-- name: SetScheduleState :exec
UPDATE schedule SET state = $2 WHERE id = $1;
