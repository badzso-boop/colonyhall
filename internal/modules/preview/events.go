package preview

// Event types.
const (
	EventPreviewRequested = "preview.requested"
	EventPreviewRunning   = "preview.running"
	EventPreviewStopped   = "preview.stopped"
	EventPreviewFailed    = "preview.failed"
)

// PreviewEvent is the audit log payload for preview lifecycle changes.
type PreviewEvent struct {
	PreviewID   string `json:"preview_id"`
	AgentID     string `json:"agent_id"`
	HostID      string `json:"host_id,omitempty"`
	Name        string `json:"name"`
	ComposePath string `json:"compose_path,omitempty"`
	State       string `json:"state"`
	URL         string `json:"url,omitempty"`
	Error       string `json:"error,omitempty"`
	StoppedBy   string `json:"stopped_by,omitempty"`
}
