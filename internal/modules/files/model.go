// Package files manages file uploads, content-addressed storage and message attachments.
package files

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode"

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
}

// Attachment is a file attached to a message.
type Attachment struct {
	ID   ids.ID
	Name string
	Mime string
	Size int64
}

// Limits.
const (
	MaxFileNameLen      = 200
	DefaultMaxUploadMB  = 25
	MaxTextDownloadSize = 64 * 1024 // 64 KB for MCP text download
)

// Errors.
var (
	ErrNotFound        = errors.New("file not found")
	ErrForbidden       = errors.New("access denied to file")
	ErrFileTooLarge    = errors.New("file exceeds maximum upload size")
	ErrEmptyContent    = errors.New("file content is empty")
	ErrChannelMismatch = errors.New("file belongs to a different channel")
	ErrNotSender       = errors.New("file was not uploaded by sender")
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
