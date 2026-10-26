-- name: GetSettings :one
SELECT workspace_id, compact_threshold_tokens, compactor_agent_id, updated_at
FROM workspace_compaction_settings
WHERE workspace_id = $1;

-- name: UpsertSettings :exec
INSERT INTO workspace_compaction_settings (workspace_id, compact_threshold_tokens, compactor_agent_id, updated_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id) DO UPDATE
SET compact_threshold_tokens = EXCLUDED.compact_threshold_tokens,
    compactor_agent_id = EXCLUDED.compactor_agent_id,
    updated_at = EXCLUDED.updated_at;

-- name: GetChannelThreshold :one
SELECT compact_threshold_tokens
FROM channel
WHERE id = $1;

-- name: SetChannelThreshold :exec
UPDATE channel
SET compact_threshold_tokens = $2
WHERE id = $1;

-- name: GetUnit :one
SELECT id, workspace_id, channel_id, thread_root_id, approx_tokens_since, messages_since, last_compaction_id, updated_at
FROM compaction_unit
WHERE channel_id = $1 AND (thread_root_id = $2 OR (thread_root_id IS NULL AND $2 IS NULL));

-- name: InsertUnit :exec
INSERT INTO compaction_unit (id, workspace_id, channel_id, thread_root_id, approx_tokens_since, messages_since, last_compaction_id, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: UpdateUnit :exec
UPDATE compaction_unit
SET approx_tokens_since = $2,
    messages_since = $3,
    last_compaction_id = $4,
    updated_at = $5
WHERE id = $1;

-- name: InsertCompaction :exec
INSERT INTO compaction (
    id, workspace_id, channel_id, thread_root_id, up_to_message_id, previous_compaction_id,
    trigger, requested_by, compactor_agent_id, state, summary_file_id, system_message_id,
    messages_count, approx_tokens_in, summary_tokens, error_code, created_at, completed_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12,
    $13, $14, $15, $16, $17, $18
);

-- name: CompactionByID :one
SELECT id, workspace_id, channel_id, thread_root_id, up_to_message_id, previous_compaction_id,
       trigger, requested_by, compactor_agent_id, state, summary_file_id, system_message_id,
       messages_count, approx_tokens_in, summary_tokens, error_code, created_at, completed_at
FROM compaction
WHERE id = $1;

-- name: LatestDoneCompaction :one
SELECT id, workspace_id, channel_id, thread_root_id, up_to_message_id, previous_compaction_id,
       trigger, requested_by, compactor_agent_id, state, summary_file_id, system_message_id,
       messages_count, approx_tokens_in, summary_tokens, error_code, created_at, completed_at
FROM compaction
WHERE channel_id = $1
  AND (thread_root_id = $2 OR (thread_root_id IS NULL AND $2 IS NULL))
  AND state = 'done'
ORDER BY created_at DESC
LIMIT 1;

-- name: PendingCompaction :one
SELECT id, workspace_id, channel_id, thread_root_id, up_to_message_id, previous_compaction_id,
       trigger, requested_by, compactor_agent_id, state, summary_file_id, system_message_id,
       messages_count, approx_tokens_in, summary_tokens, error_code, created_at, completed_at
FROM compaction
WHERE channel_id = $1
  AND (thread_root_id = $2 OR (thread_root_id IS NULL AND $2 IS NULL))
  AND state = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- name: FailPendingCompaction :exec
UPDATE compaction
SET state = 'failed',
    error_code = $2,
    completed_at = $3
WHERE id = $1 AND state = 'pending';

-- name: CompleteCompaction :exec
UPDATE compaction
SET state = 'done',
    summary_file_id = $2,
    system_message_id = $3,
    messages_count = $4,
    approx_tokens_in = $5,
    summary_tokens = $6,
    completed_at = $7
WHERE id = $1 AND state = 'pending';

-- name: CompactionIDsBySystemMessageIDs :many
SELECT system_message_id, id
FROM compaction
WHERE system_message_id = ANY($1::uuid[]);
