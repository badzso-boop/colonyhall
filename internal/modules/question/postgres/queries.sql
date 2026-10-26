-- name: InsertQuestion :one
INSERT INTO question (
    id, number, workspace_id, asked_by, to_actor_id, to_human,
    channel_id, thread_root_id, text, options, default_option,
    deadline, state, created_at, allow_expire
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15
) RETURNING *;

-- name: NextQuestionNumber :one
SELECT COALESCE(MAX(number), 0) + 1 AS next_number
FROM question
WHERE workspace_id = $1;

-- name: CountOpenByThreadAsker :one
SELECT COUNT(*)
FROM question
WHERE thread_root_id = $1 AND asked_by = $2 AND state IN ('open', 'escalated');

-- name: GetQuestionByID :one
SELECT * FROM question WHERE id = $1;

-- name: GetQuestionByNumber :one
SELECT * FROM question WHERE workspace_id = $1 AND number = $2;

-- name: ListQuestions :many
SELECT * FROM question
WHERE workspace_id = $1
  AND (sqlc.narg('state')::varchar IS NULL OR state = sqlc.narg('state'))
ORDER BY created_at DESC;

-- name: AnswerQuestion :one
UPDATE question
SET state = 'answered',
    answer = $2,
    answered_by = $3,
    answered_at = $4
WHERE id = $1 AND state IN ('open', 'escalated')
RETURNING *;

-- name: WithdrawQuestion :one
UPDATE question
SET state = 'withdrawn',
    withdrawn_at = $2
WHERE id = $1 AND state IN ('open', 'escalated')
RETURNING *;

-- name: EscalateQuestion :one
UPDATE question
SET state = 'escalated',
    escalated_at = $2
WHERE id = $1 AND state = 'open'
RETURNING *;

-- name: ExpireQuestion :one
UPDATE question
SET state = 'expired',
    expired_at = $2
WHERE id = $1 AND state IN ('open', 'escalated')
RETURNING *;

-- name: ReopenQuestion :one
UPDATE question
SET state = 'open',
    answer = NULL,
    answered_by = NULL,
    answered_at = NULL
WHERE id = $1
RETURNING *;

-- name: ListDueQuestions :many
SELECT * FROM question
WHERE state = 'open'
  AND (to_human = false OR allow_expire = true)
  AND (
    (to_human = false AND created_at <= $1)
    OR (deadline IS NOT NULL AND deadline <= $2)
  )
ORDER BY created_at ASC
LIMIT $3;
