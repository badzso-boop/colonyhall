package lease

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"colonyhall/internal/platform/ids"
)

var (
	ErrInvalidResource = errors.New("invalid resource name (must be 1-100 characters, containing only a-z, 0-9, ., _, :, /, -)")
	ErrInvalidTTL      = errors.New("invalid ttl_minutes (must be between 1 and 240)")
	ErrNoteTooLong     = errors.New("note must be at most 200 characters")
	ErrLeaseNotFound   = errors.New("lease not found")
	ErrForbidden       = errors.New("forbidden")
	ErrNotHolder       = errors.New("only the holder may release this lease")
	ErrBadState        = errors.New("state must be active or all")
	ErrBadSince        = errors.New("since must be within the last 7 days")
)

var resourceRegexp = regexp.MustCompile(`^[a-z0-9._:/-]+$`)

// NormalizeAndValidateResource trims, lowercases, and validates a resource name.
func NormalizeAndValidateResource(raw string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(raw))
	if len(norm) == 0 || len(norm) > 100 {
		return "", ErrInvalidResource
	}
	if !resourceRegexp.MatchString(norm) {
		return "", ErrInvalidResource
	}
	return norm, nil
}

// Lease represents a lease on a shared resource.
type Lease struct {
	ID           ids.ID
	WorkspaceID  ids.ID
	Resource     string
	HolderID     ids.ID
	HolderHandle string
	Note         string
	AcquiredAt   time.Time
	ExpiresAt    time.Time
	ReleasedAt   *time.Time
	ReleasedBy   *ids.ID
}

// IsActive returns whether the lease is active at the given time.
func (l Lease) IsActive(now time.Time) bool {
	return l.ReleasedAt == nil && l.ExpiresAt.After(now)
}
