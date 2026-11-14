// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package files

// Event types.
const (
	EventFileUploaded             = "file.uploaded"
	EventFileFetchRequested       = "file.fetch_requested"
	EventFileFetchCompleted       = "file.fetch_completed"
	EventFileFetchFailed          = "file.fetch_failed"
	EventFileFetchSettingsUpdated = "file.fetch_settings_updated"
	// Round 54.
	EventFileDeleted = "file.deleted"
	// EventStorageStateChanged is workspace-wide (no channel): the
	// workspace.* prefix lets it reach the guards, which pause or resume
	// the agents on it.
	EventStorageStateChanged = "workspace.storage_state_changed"
	EventStorageQuotaSet     = "workspace.storage_quota_set"
)

// DeletedPayload is the payload of file.deleted: no name, no size.
type DeletedPayload struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
}

// StorageStatePayload is the payload of workspace.storage_state_changed:
// numbers only, never people or files.
type StorageStatePayload struct {
	State         string `json:"state"`
	PreviousState string `json:"previous_state"`
	UsedBytes     int64  `json:"used_bytes"`
	QuotaBytes    int64  `json:"quota_bytes"`
}

// QuotaSetPayload is the payload of workspace.storage_quota_set.
type QuotaSetPayload struct {
	QuotaBytes int64 `json:"quota_bytes"`
}

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
