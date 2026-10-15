// Package runs keeps what agents' runs leave behind. For now: error
// details, which the chat must not show (it only gets "I ran into an error"
// and a reference) and which only humans may read.
package runs

import (
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"colonyhall/internal/platform/ids"
)

// Limits.
const (
	MaxSummary = 200
	MaxDetail  = 8000
)

// Error is one reported run error.
type Error struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	AgentID      ids.ID
	ChannelID    ids.ID
	ThreadRootID ids.ID
	Summary      string
	Detail       string
	CreatedAt    time.Time
}

// Errors.
var (
	ErrForbidden = errors.New("not allowed")
	ErrEmpty     = errors.New("summary is required")
)

// secretPattern matches Colonyhall and common provider tokens, so a client
// that echoes one in its error output does not leak it into the store.
var secretPattern = regexp.MustCompile(`(col_|colh_|sk-ant-[a-z0-9]*-|sk-|AIza|ya29\.|ghp_|github_pat_)[A-Za-z0-9_\-\.]{8,}`)

// Redact masks secrets in s.
func Redact(s string) string { return secretPattern.ReplaceAllString(s, "${1}[redacted]") }

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
