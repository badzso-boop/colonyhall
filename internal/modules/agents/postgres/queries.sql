-- name: CreateInstructions :exec
INSERT INTO agent_instructions (agent_id, workspace_id, version, text, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: InstructionsByAgent :one
SELECT * FROM agent_instructions WHERE agent_id = $1 ORDER BY version DESC LIMIT 1;
