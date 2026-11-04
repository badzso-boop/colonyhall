// Package hosts owns the machines that run a guard (`colony guard`): their
// pairing, which agents run on which machine, and which folders each agent
// may use there. The machine owner's root folders (guard.yaml) are the
// upper bound; the server can only grant folders inside them, and the
// guard re-checks every path itself.
package hosts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"path"
	"strings"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/ids"
)

// Host states.
const (
	StateApproved = "approved"
	StateRevoked  = "revoked"
)

// Grant modes.
const (
	ModeRead  = "read"
	ModeWrite = "write"
)

// Lifetimes.
const (
	PairingTTL    = 10 * time.Minute
	AgentTokenTTL = time.Hour
)

// PairingCode lets one guard register; it is shown once to the admin.
type PairingCode struct {
	ID          ids.ID
	WorkspaceID ids.ID
	CodeHash    []byte
	CreatedBy   ids.ID
	CreatedAt   time.Time
	ExpiresAt   time.Time
	UsedAt      *time.Time
}

// Host is a machine running a guard.
type Host struct {
	ID          ids.ID
	WorkspaceID ids.ID
	Name        string
	Color       string // a palette hue (platform/palette)
	TokenHash   []byte
	State       string
	CreatedBy   ids.ID
	CreatedAt   time.Time
	LastSeenAt  *time.Time
	Report      Report
	// OwnerID is the person the machine belongs to (round 38), ids.Nil
	// when nobody's: they see it on the Machines page and set its Claude login.
	OwnerID ids.ID
}

// Report is what the guard says about its machine. It is informational
// for the admin; the guard enforces its own roots regardless.
type Report struct {
	GuardVersion     string                  `json:"guard_version,omitempty"`
	OS               string                  `json:"os,omitempty"`
	Roots            []string                `json:"roots"`
	Clients          []Client                `json:"clients"`
	Sandbox          string                  `json:"sandbox,omitempty"` // e.g. "bwrap 0.9.0"; empty: none
	AllowUnsandboxed bool                    `json:"allow_unsandboxed"`
	ExchangeDir      string                  `json:"exchange_dir,omitempty"`
	ExchangeOK       bool                    `json:"exchange_ok"`
	Rejected         []Rejected              `json:"rejected"`
	Worktrees        []Worktree              `json:"worktrees,omitempty"`
	ClaudeLimits     *apispec.ClaudeLimits   `json:"claude_limits,omitempty"`
	AgyLimits        []apispec.AgyLimit      `json:"agy_limits,omitempty"`
	AgyUsage         []apispec.AgyUsageGroup `json:"agy_usage,omitempty"`
	UsageCheckedAt   *time.Time              `json:"usage_checked_at,omitempty"`
}

// Worktree describes a git worktree reported by the guard.
type Worktree struct {
	AgentID string `json:"agent_id"`
	Path    string `json:"path"`
	Branch  string `json:"branch"`
}

// Client is an AI client found on the machine.
type Client struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// Rejected is a granted path the guard refused, with the reason.
type Rejected struct {
	AgentID string `json:"agent_id"`
	Path    string `json:"path"`
	Reason  string `json:"reason"`
}

// Agent assignment modes.
const (
	AgentModeDirect  = "direct"
	AgentModeCompact = "compact"
)

// Approval modes for an agent on a host.
const (
	ApprovalsNone = "none"
	ApprovalsChat = "chat"
)

// Assignment says which host runs an agent, and how.
type Assignment struct {
	AgentID          ids.ID
	HostID           ids.ID
	WorkspaceID      ids.ID
	Client           string
	Model            string
	WorkModel        string
	AllowWork        bool
	PermissionMode   string
	Mode             string
	RootDir          string
	Sandbox          bool
	Approvals        string
	MaxSessionTokens int
	DailyTokenLimit  int64
	CreatedBy        ids.ID
	CreatedAt        time.Time
	WorktreeRepo     string
	AllowPreview     bool
	AllowWebRead     bool
}

// HostPreview is what a guard needs to know about an active/requested preview.
type HostPreview struct {
	ID          ids.ID    `json:"id"`
	AgentID     ids.ID    `json:"agent_id"`
	Name        string    `json:"name"`
	ComposePath string    `json:"compose_path"`
	State       string    `json:"state"`
	URL         string    `json:"url,omitempty"`
	Error       string    `json:"error,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Grant lets an agent use a folder on its host.
type Grant struct {
	ID          ids.ID
	WorkspaceID ids.ID
	AgentID     ids.ID
	HostID      ids.ID
	Path        string
	Mode        string
	CreatedBy   ids.ID
	CreatedAt   time.Time
	RevokedAt   *time.Time
}

// AgentConfig is one agent of a host's config.
type AgentConfig struct {
	Assignment
	Handle string
	Grants []Grant
}

// Errors.
var (
	ErrForbidden            = errors.New("not allowed")
	ErrBadCode              = errors.New("invalid or expired pairing code")
	ErrBadHostToken         = errors.New("invalid or revoked host token")
	ErrBadPath              = errors.New("path must be absolute and clean (no '..'), at most 1024 characters")
	ErrBadMode              = errors.New("mode must be read or write")
	ErrBadAgentMode         = errors.New("mode must be direct or compact")
	ErrBadRootDir           = errors.New("root_dir is required unless the agent is sandboxed: give a folder, or choose compact mode with sandbox")
	ErrWorktreeDirectOnly   = errors.New("worktree_repo is only supported for direct agents or sandboxed compact agents")
	ErrPreviewFullAccess    = errors.New("allow_preview needs a full-access agent (direct mode, no sandbox)")
	ErrWebReadSandboxedOnly = errors.New("allow_web_read is only available for sandboxed agents")
	ErrBadClient            = errors.New("client must be claude-code or antigravity")
	ErrBadApprovals         = errors.New("approvals must be none or chat")
	ErrApprovalsChatClient  = errors.New("approvals=chat requires client claude-code")
	ErrBadMaxSessionTokens  = errors.New("max_session_tokens must be non-negative")
	ErrBadDailyTokenLimit   = errors.New("daily_token_limit must be non-negative")
	ErrBadName              = errors.New("host name must be 1-64 characters")
	ErrBadColor             = errors.New("color must be one of the palette hues")
	ErrOutsideRoots         = errors.New("the path is outside the folders the machine reported (its guard.yaml roots)")
	ErrClientNotReported    = errors.New("the machine has not reported this client (is its guard running?)")
	ErrNotAssigned          = errors.New("the agent is not assigned to this host")
	ErrNotAgent             = errors.New("only agents can be assigned to a host")
	ErrHostRevoked          = errors.New("the host is revoked")
	ErrBadState             = errors.New("state must be approved or revoked")
)

// Clients the guard knows.
var Clients = map[string]bool{"claude-code": true, "antigravity": true}

// ValidatePath checks a granted path's form. The guard re-checks it on the
// machine (symlinks, roots, deny list).
func ValidatePath(p string) error {
	if len(p) == 0 || len(p) > 1024 || !strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.ContainsRune(p, 0) {
		return ErrBadPath
	}
	return nil
}

// Within reports whether p is root or inside it (both clean and absolute).
func Within(p, root string) bool {
	return p == root || root == "/" || strings.HasPrefix(p, root+"/")
}

const (
	hostTokenPrefix = "colh_"
	codeAlphabet    = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/I
)

// NewHostSecret returns a host token secret.
func NewHostSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hostTokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// NewPairingCode returns a 12-character code (60 bits) as XXXX-XXXX-XXXX.
func NewPairingCode() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(codeAlphabet[int(c)%len(codeAlphabet)])
	}
	return sb.String(), nil
}

// NormalizeCode makes typing forgiving: case and dashes do not matter.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(code))
}

// Hash returns the stored form of a secret or code.
func Hash(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}
