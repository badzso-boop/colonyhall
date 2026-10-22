package files

// Event types.
const (
	EventFileUploaded             = "file.uploaded"
	EventFileFetchRequested       = "file.fetch_requested"
	EventFileFetchCompleted       = "file.fetch_completed"
	EventFileFetchFailed          = "file.fetch_failed"
	EventFileFetchSettingsUpdated = "file.fetch_settings_updated"
)

// UploadedPayload is the payload of file.uploaded.
type UploadedPayload struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UploadedBy  string `json:"uploaded_by"`
	ChannelID   string `json:"channel_id"`
	Name        string `json:"name"`
	Mime        string `json:"mime"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

// FetchRequestedPayload is the payload of file.fetch_requested (no file content!).
type FetchRequestedPayload struct {
	RequestID string `json:"request_id"`
	AgentID   string `json:"agent_id"`
	MessageID string `json:"message_id"`
}

// FetchCompletedPayload is the payload of file.fetch_completed.
type FetchCompletedPayload struct {
	RequestID string `json:"request_id"`
	AgentID   string `json:"agent_id"`
	MessageID string `json:"message_id"`
	FileID    string `json:"file_id"`
}

// FetchFailedPayload is the payload of file.fetch_failed.
type FetchFailedPayload struct {
	RequestID string `json:"request_id"`
	AgentID   string `json:"agent_id"`
	MessageID string `json:"message_id"`
	ErrorCode string `json:"error_code"`
}

// FetchSettingsUpdatedPayload is the payload of file.fetch_settings_updated.
type FetchSettingsUpdatedPayload struct {
	MaxImageMB int `json:"max_image_mb"`
	MaxFileMB  int `json:"max_file_mb"`
}
