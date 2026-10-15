// Package identity owns workspaces, actors (humans and agents) and API tokens.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"time"

	"colonyhall/internal/platform/ids"
)

// Kind tells humans and agents apart.
type Kind string

// Actor kinds.
const (
	KindHuman Kind = "human"
	KindAgent Kind = "agent"
)

// Role is an actor's workspace-wide role.
type Role string

// Roles, from most to least privileged.
const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleMember   Role = "member"
	RoleObserver Role = "observer"
)

// Workspace isolates everything; phase 0 has exactly one.
type Workspace struct {
	ID        ids.ID
	Name      string
	CreatedAt time.Time
	// EmergencyStopAt is set while the emergency stop is on: every agent is
	// halted (no writes, no wake-ups) until an admin clears it.
	EmergencyStopAt *time.Time
	EmergencyStopBy ids.ID
}

// Actor states.
const (
	StateActive  = "active"
	StatePaused  = "paused"  // may authenticate and read, may not write
	StateStopped = "stopped" // tokens and sessions rejected
)

// Actor is anyone who can read or write: a human or an AI agent.
type Actor struct {
	ID          ids.ID
	WorkspaceID ids.ID
	Kind        Kind
	Handle      string
	DisplayName string
	Role        Role
	State       string
	CreatedAt   time.Time

	// Halted is computed when the actor is loaded for a request, never
	// stored: the actor is paused, or it is an agent while the workspace's
	// emergency stop is on. Halted actors may read but not write.
	Halted bool
}

// IsAdmin reports whether the actor may see and control everything.
func (a Actor) IsAdmin() bool { return a.Role == RoleOwner || a.Role == RoleAdmin }

// Rank orders roles: owner > admin > member > observer.
func (r Role) Rank() int {
	switch r {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleMember:
		return 1
	default:
		return 0
	}
}

// CanManage reports whether a may control target's account (password,
// tokens, state): only human admins, and only for actors of strictly lower
// rank, so an admin can never take over an owner or another admin.
func (a Actor) CanManage(target Actor) bool {
	return a.Kind == KindHuman && a.IsAdmin() && a.WorkspaceID == target.WorkspaceID &&
		a.Role.Rank() > target.Role.Rank()
}

// Token is a stored API token; only the hash of the secret is kept.
type Token struct {
	ID        ids.ID
	ActorID   ids.ID
	Hash      []byte
	Label     string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// Errors returned by the identity service.
var (
	ErrInvalidHandle = errors.New("handle must be 2-32 characters: lowercase letters, digits and '-'")
	ErrUnauthorized  = errors.New("invalid or revoked token")
	ErrForbidden     = errors.New("not allowed")
	ErrInvalidKind   = errors.New("kind must be \"human\" or \"agent\"")
	ErrInvalidState  = errors.New("state must be active, paused or stopped")
	ErrHalted        = errors.New("writing is paused for this actor (paused or emergency stop)")
)

var handlePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)

// ValidateHandle checks the @handle format.
func ValidateHandle(h string) error {
	if !handlePattern.MatchString(h) {
		return ErrInvalidHandle
	}
	return nil
}

// tokenPrefix makes Colonyhall tokens recognisable, e.g. for secret scanners.
const tokenPrefix = "col_"

// NewSecret returns a new random token secret.
func NewSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// HashSecret returns the stored form of a token secret.
func HashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
