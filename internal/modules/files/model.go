// Package files manages file uploads, content-addressed storage and message attachments.
package files

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"colonyhall/internal/modules/conversation"
	"colonyhall/internal/platform/ids"
)

// File represents uploaded file metadata.
type File struct {
	ID          ids.ID
	WorkspaceID ids.ID
	UploadedBy  ids.ID
	ChannelID   ids.ID
	Name        string
	Mime        string
	Size        int64
	SHA256      string
	CreatedAt   time.Time
	HumanOnly   bool
}

// FileFetchState represents the state of a file fetch request.
type FileFetchState string

const (
	FileFetchPending FileFetchState = "pending"
	FileFetchDone    FileFetchState = "done"
	FileFetchFailed  FileFetchState = "failed"
)

// FileFetchErrorCode represents reasons why a file fetch failed.
type FileFetchErrorCode string

const (
	FileFetchErrorHostOffline FileFetchErrorCode = "host_offline"
	FileFetchErrorNotFound    FileFetchErrorCode = "not_found"
	FileFetchErrorDenied      FileFetchErrorCode = "denied"
	FileFetchErrorTooLarge    FileFetchErrorCode = "too_large"
	FileFetchErrorNotAFile    FileFetchErrorCode = "not_a_file"
	FileFetchErrorTimeout     FileFetchErrorCode = "timeout"
)

// FileFetchRequest tracks a human request to fetch a local file linked by an agent.
type FileFetchRequest struct {
	ID          ids.ID
	WorkspaceID ids.ID
	MessageID   ids.ID
	ChannelID   ids.ID
	AgentID     ids.ID
	RequestedBy ids.ID
	Path        string
	State       FileFetchState
	FileID      ids.ID // ids.Nil if not completed
	ErrorCode   FileFetchErrorCode
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// FileFetchSettings holds workspace-level limits for file fetches.
type FileFetchSettings struct {
	WorkspaceID ids.ID
	MaxImageMB  int
	MaxFileMB   int
	UpdatedAt   time.Time
}

// Attachment is a file attached to a message.
type Attachment = conversation.Attachment

// Limits.
const (
	MaxFileNameLen      = 200
	DefaultMaxUploadMB  = 25
	MaxTextDownloadSize = 64 * 1024 // 64 KB for MCP text download

	DefaultMaxImageMB = 200
	DefaultMaxFileMB  = 100
	MinLimitMB        = 1
	MaxLimitMB        = 2048
)

// Errors.
var (
	ErrNotFound            = errors.New("file not found")
	ErrForbidden           = errors.New("access denied to file")
	ErrFileTooLarge        = errors.New("file exceeds maximum upload size")
	ErrEmptyContent        = errors.New("file content is empty")
	ErrChannelMismatch     = errors.New("file belongs to a different channel")
	ErrNotSender           = errors.New("file was not uploaded by sender")
	ErrPathNotInMessage    = errors.New("path does not occur in message body")
	ErrAuthorNotAgent      = errors.New("message author is not an agent")
	ErrAgentNotAssigned    = errors.New("agent has no host assignment")
	ErrInvalidLimit        = errors.New("limit must be between 1 and 2048 MB")
	ErrInvalidRequestState = errors.New("file fetch request is not pending")
	ErrUnknownErrorCode    = errors.New("unknown error code")
)

// SanitizeFileName cleans a user-provided filename:
// - strips directory paths (/ and \)
// - strips control characters and traversal sequences (..)
// - limits length to MaxFileNameLen
// - falls back to "attachment" if empty
func SanitizeFileName(name string) string {
	// Normalize path separators to forward slash
	name = strings.ReplaceAll(name, "\\", "/")
	// Extract base filename (removes path components)
	name = filepath.Base(name)
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, ".")

	// Filter out non-printable or dangerous characters
	var sb strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r == 0 || unicode.IsControl(r) {
			continue
		}
		sb.WriteRune(r)
	}
	clean := strings.TrimSpace(sb.String())
	// Remove any leading/trailing dots or spaces
	clean = strings.Trim(clean, ". ")

	if clean == "" || clean == "." || clean == ".." {
		clean = "attachment"
	}

	if len(clean) > MaxFileNameLen {
		ext := filepath.Ext(clean)
		if len(ext) > 20 {
			ext = ""
		}
		base := clean[:MaxFileNameLen-len(ext)]
		clean = base + ext
	}
	return clean
}
