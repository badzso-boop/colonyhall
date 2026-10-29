-- name: InsertTask :exec
INSERT INTO planner_task (id, workspace_id, owner_id, title, description, status, position, started_at, done_at,
                          status_changed_at, deleted_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: UpdateTask :execrows
UPDATE planner_task
SET title = $2, description = $3, status = $4, position = $5, started_at = $6, done_at = $7,
    status_changed_at = $8, deleted_at = $9, updated_at = $10
WHERE id = $1;

-- name: TaskByID :one
SELECT * FROM planner_task WHERE id = $1;

-- name: Tasks :many
SELECT * FROM planner_task
WHERE owner_id = $1 AND deleted_at IS NULL
  AND (status <> 'done' OR done_at >= sqlc.arg(done_since))
ORDER BY position, created_at;

-- name: MinPosition :one
SELECT coalesce(min(position), 0)::double precision AS min_pos, count(*) AS n
FROM planner_task WHERE owner_id = $1 AND deleted_at IS NULL;

-- name: Access :one
SELECT * FROM planner_access WHERE owner_id = $1;

-- name: PutAccess :exec
INSERT INTO planner_access (owner_id, workspace_id, task_all, task_agent_ids, cal_agent_ids, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (owner_id) DO UPDATE
SET task_all = EXCLUDED.task_all, task_agent_ids = EXCLUDED.task_agent_ids,
    cal_agent_ids = EXCLUDED.cal_agent_ids, updated_at = EXCLUDED.updated_at;

-- name: AccessList :many
SELECT * FROM planner_access WHERE workspace_id = $1 ORDER BY owner_id;

-- name: InsertCalendar :exec
INSERT INTO planner_calendar (id, workspace_id, owner_id, name, url, colour, last_fetched_at, last_error, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: CalendarByID :one
SELECT * FROM planner_calendar WHERE id = $1;

-- name: Calendars :many
SELECT * FROM planner_calendar WHERE owner_id = $1 ORDER BY created_at, id;

-- name: CalendarsDue :many
SELECT * FROM planner_calendar
WHERE last_fetched_at IS NULL OR last_fetched_at < $1
ORDER BY created_at, id;

-- name: DeleteCalendar :exec
DELETE FROM planner_calendar WHERE id = $1;

-- name: SetCalendarFetch :execrows
UPDATE planner_calendar SET last_fetched_at = $2, last_error = $3 WHERE id = $1;

-- name: DeleteEvents :exec
DELETE FROM planner_calendar_event WHERE calendar_id = $1;

-- name: InsertEvent :exec
INSERT INTO planner_calendar_event (calendar_id, owner_id, title, starts_at, ends_at, all_day, location, description)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: Events :many
SELECT * FROM planner_calendar_event
WHERE owner_id = $1
  AND ((ends_at > sqlc.arg(from_at) AND starts_at < sqlc.arg(to_at))
       OR (starts_at = ends_at AND starts_at >= sqlc.arg(from_at) AND starts_at < sqlc.arg(to_at)))
ORDER BY starts_at;
