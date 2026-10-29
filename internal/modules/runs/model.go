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

// Phase of an agent run.
const (
	PhaseDecide = "decide"
	PhaseWork   = "work"
	PhaseDirect = "direct"
)

// Status of an agent run.
const (
	StatusOk     = "ok"
	StatusError  = "error"
	StatusHalted = "halted"
)

// Max values from spec.
const (
	MaxTokens     = 1_000_000_000
	MaxDurationMS = 24 * 60 * 60 * 1000 // 24 hours in ms
)

// Step is one recorded step in an agent run.
type Step struct {
	Kind       string    `json:"kind"`
	Label      string    `json:"label"`
	Path       string    `json:"path,omitempty"`
	State      string    `json:"state"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs int64     `json:"duration_ms,omitempty"`
}

// Run is one recorded agent run.
type Run struct {
	ID               ids.ID
	WorkspaceID      ids.ID
	AgentID          ids.ID
	HostID           ids.ID
	Phase            string
	Client           string
	Model            string
	SessionID        string
	ChannelID        ids.ID
	ThreadRootID     ids.ID
	Items            int
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	// ContextTokens is the session's context size after the run as the
	// client measured it; 0 when unknown.
	ContextTokens  int64
	DurationMS     int
	Status         string
	ErrorID        ids.ID
	CreatedAt      time.Time
	WebReads       []string
	Steps          []Step
	ReplyMessageID ids.ID
}

// DayUsage is the daily aggregated token usage and run count for one agent in Europe/Budapest.
type DayUsage struct {
	AgentID          ids.ID
	Day              string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	TotalTokens      int64
	RunCount         int
	ErrorCount       int
}

// AgentBudget is the daily token budget status for one agent in Europe/Budapest.
type AgentBudget struct {
	Limit     int64
	UsedToday int64
	Remaining int64
	ResetsAt  time.Time
}

// Errors.
var (
	ErrForbidden   = errors.New("not allowed")
	ErrEmpty       = errors.New("summary is required")
	ErrInvalidRun  = errors.New("invalid run data")
	ErrInvalidDays = errors.New("days must be between 1 and 90")
)

// Budapest timezone used for daily aggregations.
var Budapest = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Budapest")
	if err != nil {
		panic("failed to load Europe/Budapest timezone: " + err.Error())
	}
	return loc
}()

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
