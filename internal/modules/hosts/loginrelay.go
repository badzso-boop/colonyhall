package hosts

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Claude login relay (round 38): a person sets the Claude Code login of a
// machine they own (the token from `claude setup-token`, for their own
// subscription). The token travels PWA → server → that machine's guard and
// lives on the server only in this relay's memory, until the guard fetches
// it once. It is never stored, logged, audited or put in an event; the
// audit records only who set it on which machine, after the guard saved it.

// Events of the login relay (payloads never carry the token).
const (
	EventClaudeLoginSet     = "host.claude_login_set"
	EventClaudeLoginCleared = "host.claude_login_cleared"
)

// Login ops.
const (
	LoginOpSet   = "set"
	LoginOpClear = "clear"
)

// Limits of the login relay.
const (
	// LoginTimeout covers the guard's check (`claude -p /usage`).
	LoginTimeout = 90 * time.Second
	// LoginAttempts per person in LoginWindow.
	LoginAttempts = 5
	LoginWindow   = 10 * time.Minute
)

// Login relay errors.
var (
	ErrLoginCheckFailed   = errors.New("claude_login_check_failed")
	ErrLoginNotConfigured = errors.New("claude_login_not_configured: the machine's guard.yaml has no claude_token_file")
	ErrLoginWriteFailed   = errors.New("claude_login_write_failed")
	ErrLoginTimeout       = errors.New("the machine did not answer in time")
	ErrLoginTooMany       = errors.New("too many Claude login attempts; try again in a few minutes")
	ErrBadLoginToken      = errors.New("the token must be the single line `claude setup-token` printed")
)

// LoginEvent is the audit payload of the login events: who and which
// machine (the actor is the event's author), never the token.
type LoginEvent struct {
	HostID string `json:"host_id"`
	Name   string `json:"name"`
}

// LoginResult is the guard's answer.
type LoginResult struct {
	OK        bool
	ErrorCode string // check_failed, write_failed, not_configured
	SetAt     *time.Time
}

// HostLoginPusher wakes the guard of a host (the gateway's /ws/host).
type HostLoginPusher interface {
	PushHostLoginRequest(hostID ids.ID, requestID string) bool
}

type loginPending struct {
	id     string
	hostID ids.ID
	op     string
	token  string // cleared once the guard fetched it
	result chan LoginResult
}

// LoginRelay relays Claude login changes to guards. Safe for concurrent use.
type LoginRelay struct {
	Hosts   *Service
	Pusher  HostLoginPusher
	Timeout time.Duration // LoginTimeout when zero

	mu       sync.Mutex
	pending  map[string]*loginPending
	expired  map[string]time.Time
	attempts map[ids.ID][]time.Time
}

// mayLogin: the machine's owner or an admin, a person who is not halted.
func mayLogin(by identity.Actor, h Host) error {
	if by.Kind != identity.KindHuman || by.Halted {
		return ErrForbidden
	}
	if by.IsAdmin() || (h.OwnerID != ids.Nil && h.OwnerID == by.ID) {
		return nil
	}
	return ErrForbidden
}

// Set relays a new token and waits for the guard to check and save it.
func (r *LoginRelay) Set(ctx context.Context, by identity.Actor, hostID ids.ID, token string) (LoginResult, error) {
	token = strings.TrimSpace(token)
	if len(token) < 20 || len(token) > 1000 || strings.ContainsAny(token, " \t\r\n") {
		return LoginResult{}, ErrBadLoginToken
	}
	return r.relay(ctx, by, hostID, LoginOpSet, token)
}

// Clear relays the removal of the token file.
func (r *LoginRelay) Clear(ctx context.Context, by identity.Actor, hostID ids.ID) error {
	_, err := r.relay(ctx, by, hostID, LoginOpClear, "")
	return err
}

func (r *LoginRelay) relay(ctx context.Context, by identity.Actor, hostID ids.ID, op, token string) (LoginResult, error) {
	if by.Kind != identity.KindHuman {
		return LoginResult{}, ErrForbidden
	}
	h, err := r.Hosts.hostIn(ctx, by.WorkspaceID, hostID)
	if err != nil {
		return LoginResult{}, err
	}
	if err := mayLogin(by, h); err != nil {
		// Another person's machine is not theirs to know about.
		if !by.IsAdmin() {
			return LoginResult{}, db.ErrNotFound
		}
		return LoginResult{}, err
	}
	if !r.allow(by.ID) {
		return LoginResult{}, ErrLoginTooMany
	}
	if h.State != StateApproved {
		return LoginResult{}, ErrHostOffline
	}
	p := &loginPending{id: r.Hosts.IDs.New().String(), hostID: h.ID, op: op, token: token, result: make(chan LoginResult, 1)}
	r.mu.Lock()
	if r.pending == nil {
		r.pending = map[string]*loginPending{}
		r.expired = map[string]time.Time{}
	}
	r.pending[p.id] = p
	r.mu.Unlock()
	defer r.finish(p.id) // the token never outlives this call

	if r.Pusher == nil || !r.Pusher.PushHostLoginRequest(h.ID, p.id) {
		return LoginResult{}, ErrHostOffline
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = LoginTimeout
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	var res LoginResult
	select {
	case res = <-p.result:
	case <-t.C:
		return LoginResult{}, ErrLoginTimeout
	case <-ctx.Done():
		return LoginResult{}, ctx.Err()
	}
	if !res.OK {
		switch res.ErrorCode {
		case "not_configured":
			return res, ErrLoginNotConfigured
		case "write_failed":
			return res, ErrLoginWriteFailed
		default:
			return res, ErrLoginCheckFailed
		}
	}
	typ := EventClaudeLoginSet
	if op == LoginOpClear {
		typ = EventClaudeLoginCleared
	}
	err = r.Hosts.record(ctx, by.WorkspaceID, by.ID, typ, h.ID, LoginEvent{HostID: h.ID.String(), Name: h.Name},
		func(context.Context) error { return nil })
	return res, err
}

// allow counts an attempt of the person; false when over the limit.
func (r *LoginRelay) allow(actorID ids.ID) bool {
	now := r.Hosts.Clock.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attempts == nil {
		r.attempts = map[ids.ID][]time.Time{}
	}
	kept := r.attempts[actorID][:0]
	for _, at := range r.attempts[actorID] {
		if now.Sub(at) < LoginWindow {
			kept = append(kept, at)
		}
	}
	if len(kept) >= LoginAttempts {
		r.attempts[actorID] = kept
		return false
	}
	r.attempts[actorID] = append(kept, now)
	return true
}

// Request hands the guard of host h its pending request. The token is
// given out once: a second read gets the op without it.
func (r *LoginRelay) Request(h Host, id string) (op, token string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok || p.hostID != h.ID {
		return "", "", db.ErrNotFound
	}
	token, p.token = p.token, ""
	return p.op, token, nil
}

// Deliver hands the guard's answer to the waiting person.
func (r *LoginRelay) Deliver(h Host, id string, res LoginResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok {
		if _, gone := r.expired[id]; gone {
			return ErrFsGone
		}
		return db.ErrNotFound
	}
	if p.hostID != h.ID {
		return db.ErrNotFound
	}
	select {
	case p.result <- res:
	default:
	}
	return nil
}

func (r *LoginRelay) finish(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.pending[id]
	if !ok {
		return
	}
	p.token = ""
	delete(r.pending, id)
	now := r.Hosts.Clock.Now()
	r.expired[id] = now
	for k, at := range r.expired {
		if now.Sub(at) > 5*time.Minute {
			delete(r.expired, k)
		}
	}
}

// Pending reports whether any request still holds a token (tests use it
// to prove the server forgets the token).
func (r *LoginRelay) Pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, p := range r.pending {
		if p.token != "" {
			n++
		}
	}
	return n
}
