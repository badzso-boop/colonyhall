package guard

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	StepKindRead    = "read"
	StepKindWrite   = "write"
	StepKindEdit    = "edit"
	StepKindSearch  = "search"
	StepKindCommand = "command"
	StepKindWeb     = "web"
	StepKindMCP     = "mcp"
	StepKindThink   = "think"
	StepKindAnswer  = "answer"
	StepKindDenied  = "denied"
	StepKindOther   = "other"
)

const (
	StepStateActive = "active"
	StepStateDone   = "done"
)

// Step represents a single progress action of an agent run.
type Step struct {
	Kind       string        `json:"kind"`
	Label      string        `json:"label"`
	Path       string        `json:"path,omitempty"`
	State      string        `json:"state"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration,omitempty"`
	DurationMs int64         `json:"duration_ms,omitempty"`
}

var (
	reAnsi       = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	reURLUser    = regexp.MustCompile(`(https?://)(?:[^/\s:@]+:[^/\s:@]+@)`)
	reAuthHeader = regexp.MustCompile(`(?i)(Authorization:\s*)(?:Bearer\s+)?[^\s,;'\"]+`)
	reFlags      = regexp.MustCompile(`(?i)(--(?:token|password|passwd|secret|key|api[-_]?key)(?:=|\s+))[^\s'\"]+`)
	reEnvKV      = regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:TOKEN|PASSWORD|PASSWD|SECRET|KEY|AUTH)[A-Z0-9_]*=)[^\s'\"]+`)
	reGHP        = regexp.MustCompile(`\bghp_[A-Za-z0-9_]+`)
	reSK         = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`)
	reCol        = regexp.MustCompile(`\bcol_[A-Za-z0-9_-]+`)
	reAKIA       = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reJWT        = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)
	reLongHexB64 = regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b|\b[A-Za-z0-9+/=_-]{32,}\b`)
)

// Redact replaces sensitive tokens, passwords, keys, and long hashes in commands
// and labels with "***", strips control characters, and caps the result at 80 characters.
func Redact(s string) string {
	if s == "" {
		return ""
	}
	// Strip ANSI sequences (replace with space to prevent word joining)
	s = reAnsi.ReplaceAllString(s, " ")

	// Strip control characters (replace with space)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Redact known patterns
	s = reURLUser.ReplaceAllString(s, "${1}***@")
	s = reAuthHeader.ReplaceAllString(s, "${1}***")
	s = reFlags.ReplaceAllString(s, "${1}***")
	s = reEnvKV.ReplaceAllString(s, "${1}***")
	s = reGHP.ReplaceAllString(s, "***")
	s = reSK.ReplaceAllString(s, "***")
	s = reCol.ReplaceAllString(s, "***")
	s = reAKIA.ReplaceAllString(s, "***")
	s = reJWT.ReplaceAllString(s, "***")
	s = reLongHexB64.ReplaceAllString(s, "***")

	// Collapse whitespace
	s = strings.Join(strings.Fields(s), " ")

	// Cap at 80 characters
	runes := []rune(s)
	if len(runes) > 80 {
		s = string(runes[:80])
	}
	return s
}

// RelPath returns a relative path if path is inside any of the baseDirs.
// If outside all baseDirs, only the base file name is returned.
func RelPath(path string, baseDirs ...string) string {
	if path == "" {
		return ""
	}
	cleanPath := filepath.Clean(path)
	for _, base := range baseDirs {
		if base == "" {
			continue
		}
		cleanBase := filepath.Clean(base)
		rel, err := filepath.Rel(cleanBase, cleanPath)
		if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return rel
		}
		if cleanPath == cleanBase {
			return filepath.Base(cleanBase)
		}
	}
	return filepath.Base(cleanPath)
}
