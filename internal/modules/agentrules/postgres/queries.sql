-- name: InsertRules :exec
INSERT INTO agent_rules (workspace_id, version, text, created_by, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetLatestRules :one
SELECT workspace_id, version, text, created_by, created_at
FROM agent_rules
WHERE workspace_id = $1
ORDER BY version DESC
LIMIT 1;

-- name: GetRulesVersion :one
SELECT workspace_id, version, text, created_by, created_at
FROM agent_rules
WHERE workspace_id = $1 AND version = $2;

-- name: ListRulesHistory :many
SELECT version, created_by, created_at
FROM agent_rules
WHERE workspace_id = $1
ORDER BY version DESC;
