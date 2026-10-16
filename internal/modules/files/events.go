package files

// Event types.
const (
	EventFileUploaded = "file.uploaded"
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
