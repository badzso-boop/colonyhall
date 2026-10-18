package notes

import (
	"time"

	"colonyhall/internal/platform/ids"
)

// Note is an agent's notebook entry for a thread.
type Note struct {
	ID           ids.ID    `json:"id"`
	WorkspaceID  ids.ID    `json:"workspace_id"`
	AgentID      ids.ID    `json:"agent_id"`
	ThreadRootID ids.ID    `json:"thread_root_id"`
	Text         string    `json:"text"`
	CreatedAt    time.Time `json:"created_at"`
}
