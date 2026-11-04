// Package identity owns workspaces, actors (humans and agents) and API tokens.
package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"

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

	Description string
	AvatarEmoji string
	Timezone    string
	StatusText  string
	// Color is an agent's own palette hue (round 33); "" shows its machine's.
	Color string
	// EffectiveColor is computed, never stored: Color, else the agent's
	// machine colour (Service.HostColors), else "".
	EffectiveColor string
	// ArchivedAt is set when the agent was deleted (round 33): it stays as
	// the author of its messages, and its handle is free again.
	ArchivedAt *time.Time
	ArchivedBy ids.ID
	// OwnerID is the person an agent belongs to (round 38), ids.Nil when
	// nobody's. A member sees only their own agents' runs, steps, errors,
	// folders and terminals, and only the owner or an admin wakes a
	// full-access agent.
	OwnerID ids.ID

	// Halted is computed when the actor is loaded for a request, never
	// stored: the actor is paused, or it is an agent while the workspace's
	// emergency stop is on. Halted actors may read but not write.
	Halted bool
}

// SeesAgent reports whether the actor may see an agent's details (overview,
// runs, steps, errors, usage, folders, terminals): admins see every agent,
// a person only the agents they own. For any other target it is true; the
// caller checks its own rules for humans.
func (a Actor) SeesAgent(target Actor) bool {
	if a.Kind != KindHuman || a.WorkspaceID != target.WorkspaceID {
		return false
	}
	if target.Kind != KindAgent || a.IsAdmin() {
		return true
	}
	return target.OwnerID != ids.Nil && target.OwnerID == a.ID
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
// tokens, state): only human admins who are not halted, and only for actors
// of strictly lower rank, so an admin can never take over an owner or
// another admin.
func (a Actor) CanManage(target Actor) bool {
	return a.Kind == KindHuman && a.IsAdmin() && !a.Halted && a.WorkspaceID == target.WorkspaceID &&
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
	// Prefix is the token's first characters, for telling tokens apart;
	// empty on tokens made before it was kept.
	Prefix     string
	LastUsedAt *time.Time
	// HostID and ExpiresAt are set on short-lived tokens a guard requested
	// for an agent assigned to its host.
	HostID    ids.ID
	ExpiresAt *time.Time
}

// Errors returned by the identity service.
var (
	ErrInvalidHandle      = errors.New("handle must be 2-32 characters: lowercase letters, digits and '-'")
	ErrUnauthorized       = errors.New("invalid or revoked token")
	ErrForbidden          = errors.New("not allowed")
	ErrInvalidKind        = errors.New("kind must be \"human\" or \"agent\"")
	ErrInvalidRole        = errors.New("role must be owner, admin, member or observer")
	ErrInvalidState       = errors.New("state must be active, paused or stopped")
	ErrHalted             = errors.New("writing is paused for this actor (paused or emergency stop)")
	ErrInvalidTimezone    = errors.New("timezone must be a valid IANA timezone name")
	ErrInvalidDescription = errors.New("description must be at most 300 characters")
	ErrInvalidOwner       = errors.New("the owner must be a person of the workspace who is not deleted")
	ErrInvalidStatusText  = errors.New("status text must be at most 80 characters")
	ErrInvalidDisplayName = errors.New("display name must be 1 to 80 characters on one line")
	ErrInvalidAvatarEmoji = errors.New("avatar emoji must be a valid emoji")
	ErrInvalidColor       = errors.New("color must be a palette hue, and only agents have one")
	ErrNotAnAgent         = errors.New("the actor is not an agent")
	// ErrAgentConfigImmutable: an agent's configuration is set once, at
	// creation (POST /agents); a change means deleting it (DELETE
	// /agents/{agentId}) and creating a new one.
	ErrAgentConfigImmutable = errors.New("an agent's configuration cannot change after creation: create a new agent with POST /api/v1/agents and delete this one with DELETE /api/v1/agents/{agentId}")
)

const (
	DefaultTimezone   = "Europe/Budapest"
	MaxDisplayNameLen = 80
	MaxDescriptionLen = 300
	MaxStatusTextLen  = 80
	MaxAvatarEmojiLen = 32
)

// ValidateTimezone checks if tz is a valid IANA timezone.
func ValidateTimezone(tz string) error {
	if tz == "" {
		return nil
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return ErrInvalidTimezone
	}
	return nil
}

// ValidAvatarEmoji checks if e is a short emoji sequence or empty.
func ValidAvatarEmoji(e string) bool {
	if e == "" {
		return true
	}
	if utf8.RuneCountInString(e) > MaxAvatarEmojiLen {
		return false
	}
	wide := false
	for _, r := range e {
		switch {
		case r == 0x200D: // ZWJ
		case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		case r >= 0x1F3FB && r <= 0x1F3FF: // skin tone modifiers
		case (r >= '0' && r <= '9') || r == '#' || r == '*': // keycaps
		case r > 127 && (unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) || unicode.Is(unicode.Sm, r)):
			wide = true
		default:
			return false
		}
	}
	return wide
}

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
// TokenPrefix is what of a secret may be shown: the "col_" and four more
// characters, far too few to guess the rest.
func TokenPrefix(secret string) string {
	return secret[:min(len(secret), len(tokenPrefix)+4)]
}

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
