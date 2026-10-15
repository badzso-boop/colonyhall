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
}

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
}

// IsAdmin reports whether the actor may see and control everything.
func (a Actor) IsAdmin() bool { return a.Role == RoleOwner || a.Role == RoleAdmin }

// Token is a stored API token; only the hash of the secret is kept.
type Token struct {
	ID        ids.ID
	ActorID   ids.ID
	Hash      []byte
	Label     string
	CreatedAt time.Time
}

// Errors returned by the identity service.
var (
	ErrInvalidHandle = errors.New("handle must be 2-32 characters: lowercase letters, digits and '-'")
	ErrUnauthorized  = errors.New("invalid or revoked token")
	ErrForbidden     = errors.New("not allowed")
	ErrInvalidKind   = errors.New("kind must be \"human\" or \"agent\"")
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
