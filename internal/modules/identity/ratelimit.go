package identity

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"

	"colonyhall/internal/platform/clientip"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Durable login and release limits (round 40). Failed password checks are
// stored (Store.RecordAuthFailure), so a restart keeps a lockout, and they
// are counted per handle and per client IP over a sliding window. Each
// scope has its own counters: a lockout of the login never blocks the
// emergency release, and the other way round.
const (
	LimitLogin          = "login"
	LimitEmergencyClear = "emergency_clear"
	LimitPassword       = "password"
	// LimitClaudeLogin counts every relayed Claude login (hosts module),
	// not just failures: the guard, not the server, checks the token.
	LimitClaudeLogin = "claude_login"
)

// LimitRule is how many counted attempts a handle and an IP may make in a
// window before they are locked out.
type LimitRule struct {
	PerHandle int
	PerIP     int
	Window    time.Duration
}

// LimitRules are the limits per scope. The IP limit is higher than the
// handle limit: several people may share an address (an office NAT).
var LimitRules = map[string]LimitRule{
	LimitLogin:          {PerHandle: 5, PerIP: 20, Window: 15 * time.Minute},
	LimitEmergencyClear: {PerHandle: 5, PerIP: 20, Window: 15 * time.Minute},
	LimitPassword:       {PerHandle: 5, PerIP: 20, Window: 15 * time.Minute},
	LimitClaudeLogin:    {PerHandle: 5, PerIP: 20, Window: 10 * time.Minute},
}

// AuthFailureRetention is how long failure rows are kept (the status card
// shows the last 24 hours); the hourly maintenance deletes older ones.
const AuthFailureRetention = 7 * 24 * time.Hour

// EventLockedOut: a handle or an address reached a limit.
const EventLockedOut = "auth.locked_out"

// LockedOut is the payload of EventLockedOut. Never the password.
type LockedOut struct {
	Scope  string    `json:"scope"`
	Handle string    `json:"handle"`
	IP     string    `json:"ip,omitempty"`
	ByIP   bool      `json:"by_ip"`
	Until  time.Time `json:"until"`
}

// AuthFailure is one counted attempt.
type AuthFailure struct {
	ID ids.ID
	// WorkspaceID is the workspace when known (ids.Nil: an unknown handle).
	WorkspaceID ids.ID
	Scope       string
	Handle      string
	IP          string
	At          time.Time
	// LockedUntil is set on the attempt that reached a limit.
	LockedUntil *time.Time
}

// AuthFailureTimes are the counted attempts in a window, oldest first.
type AuthFailureTimes struct {
	ByHandle []time.Time
	ByIP     []time.Time
}

// Lockout is one lockout, for the system status card.
type Lockout struct {
	Scope  string
	Handle string
	IP     string
	At     time.Time
	Until  time.Time
}

// LockedOutError is a refused attempt; Is(ErrTooManyAttempts) (or the
// sentinel in Err) and RetryAfter for the Retry-After header.
type LockedOutError struct {
	Until time.Time
	After time.Duration
	// Err is the sentinel the error matches; nil: ErrTooManyAttempts.
	Err error
}

func (e *LockedOutError) sentinel() error {
	if e.Err != nil {
		return e.Err
	}
	return ErrTooManyAttempts
}

func (e *LockedOutError) Error() string { return e.sentinel().Error() }

// Is matches the sentinel.
func (e *LockedOutError) Is(target error) bool { return target == e.sentinel() }

// RetryAfter is how long until the next attempt is accepted.
func (e *LockedOutError) RetryAfter() time.Duration { return e.After }

// limitKey normalises a handle: handles are lower case, and a long input
// must not grow the table.
func limitKey(handle string) string {
	h := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(handle), "@")))
	if len(h) > 64 {
		h = h[:64]
	}
	return h
}

// allowlisted: an allowlisted address is never locked out
// (COLONY_LOGIN_ALLOWLIST); its attempts are still counted and audited.
func (s *Service) allowlisted(ip netip.Addr) bool {
	return clientip.Contains(s.LoginAllowlist, ip)
}

// unlockAt returns when a key with these attempts (oldest first) is below
// max again, and whether it is locked now.
func unlockAt(times []time.Time, max int, window time.Duration) (time.Time, bool) {
	if max <= 0 || len(times) < max {
		return time.Time{}, false
	}
	return times[len(times)-max].Add(window), true
}

// CheckLimit refuses an attempt while the handle or the caller's address
// is locked out in scope. The address comes from the request context
// (clientip). A store error is returned as is: better a failed login than
// an unlimited one.
func (s *Service) CheckLimit(ctx context.Context, scope, handle string) error {
	rule, ok := LimitRules[scope]
	if !ok {
		return errors.New("identity: unknown limit scope " + scope)
	}
	ip := clientip.From(ctx)
	if s.allowlisted(ip) {
		return nil
	}
	now := s.Clock.Now()
	times, err := s.Store.AuthFailures(ctx, scope, limitKey(handle), clientip.String(ip), now.Add(-rule.Window))
	if err != nil {
		return err
	}
	var until time.Time
	if u, locked := unlockAt(times.ByHandle, rule.PerHandle, rule.Window); locked {
		until = u
	}
	if u, locked := unlockAt(times.ByIP, rule.PerIP, rule.Window); locked && u.After(until) {
		until = u
	}
	if until.IsZero() {
		return nil
	}
	return &LockedOutError{Until: until, After: until.Sub(now)}
}

// LimitFailed counts one failed (or, for LimitClaudeLogin, any) attempt.
// workspaceID is the workspace the attempt belongs to when known. When the
// attempt reaches a limit, the lockout is audited in that workspace (or
// logged when there is none). Errors are logged, not returned: the caller
// is already answering with a failure.
func (s *Service) LimitFailed(ctx context.Context, scope, handle string, workspaceID ids.ID) {
	rule, ok := LimitRules[scope]
	if !ok {
		return
	}
	ip := clientip.From(ctx)
	now := s.Clock.Now()
	key, addr := limitKey(handle), clientip.String(ip)
	f := AuthFailure{ID: s.IDs.New(), WorkspaceID: workspaceID, Scope: scope, Handle: key, IP: addr, At: now}
	var byIP bool
	if !s.allowlisted(ip) {
		times, err := s.Store.AuthFailures(ctx, scope, key, addr, now.Add(-rule.Window))
		if err != nil {
			s.logger().Warn("login limit: reading failures failed", "scope", scope, "err", err)
		} else {
			times.ByHandle = append(times.ByHandle, now)
			if addr != "" {
				times.ByIP = append(times.ByIP, now)
			}
			hu, hLocked := unlockAt(times.ByHandle, rule.PerHandle, rule.Window)
			iu, iLocked := unlockAt(times.ByIP, rule.PerIP, rule.Window)
			// Only the attempt that reaches the limit records a lockout:
			// later ones are refused before they are counted.
			hLocked = hLocked && len(times.ByHandle) == rule.PerHandle
			iLocked = iLocked && len(times.ByIP) == rule.PerIP
			switch {
			case hLocked && iLocked:
				f.LockedUntil, byIP = ptr(later(hu, iu)), true
			case hLocked:
				f.LockedUntil = &hu
			case iLocked:
				f.LockedUntil, byIP = &iu, true
			}
		}
	}
	if err := s.Store.RecordAuthFailure(ctx, f); err != nil {
		s.logger().Warn("login limit: recording a failure failed", "scope", scope, "err", err)
		return
	}
	if f.LockedUntil == nil {
		return
	}
	payload := LockedOut{Scope: scope, Handle: key, IP: addr, ByIP: byIP, Until: *f.LockedUntil}
	if workspaceID == ids.Nil {
		s.logger().Warn("SECURITY: login lockout", "scope", scope, "handle", key, "ip", addr, "by_ip", byIP, "until", f.LockedUntil)
		return
	}
	system := Actor{ID: ids.Nil, WorkspaceID: workspaceID}
	if err := s.record(db.WithWorkspace(ctx, workspaceID), system, EventLockedOut, "workspace", workspaceID, payload, func(context.Context) error { return nil }); err != nil {
		s.logger().Warn("login limit: auditing a lockout failed", "scope", scope, "err", err)
	}
}

// LimitSucceeded clears the handle's count in scope after a success. The
// address's count stays: one valid account must not reset the counter of
// an address that guesses others.
func (s *Service) LimitSucceeded(ctx context.Context, scope, handle string) {
	if err := s.Store.ClearAuthFailures(ctx, scope, limitKey(handle), s.Clock.Now()); err != nil {
		s.logger().Warn("login limit: clearing failures failed", "scope", scope, "err", err)
	}
}

// Lockouts lists the workspace's lockouts since a time, newest first. In
// single-tenant mode (unattributed) the lockouts of unknown handles are
// included: there is no other workspace to hide them from.
func (s *Service) Lockouts(ctx context.Context, by Actor, since time.Time, unattributed bool) ([]Lockout, error) {
	if by.Kind != KindHuman || !by.IsAdmin() {
		return nil, ErrForbidden
	}
	out, err := s.Store.WorkspaceLockouts(ctx, by.WorkspaceID, since)
	if err != nil {
		return nil, err
	}
	if unattributed {
		more, err := s.Store.UnattributedLockouts(ctx, since)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
		slices.SortFunc(out, func(a, b Lockout) int { return b.At.Compare(a.At) })
	}
	return out, nil
}

// CleanupAuthFailures deletes failure rows older than AuthFailureRetention.
func (s *Service) CleanupAuthFailures(ctx context.Context) (int64, error) {
	return s.Store.DeleteAuthFailures(ctx, s.Clock.Now().Add(-AuthFailureRetention))
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func ptr[T any](v T) *T { return &v }
