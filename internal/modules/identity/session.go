package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"sync"
	"time"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// Session is a logged-in browser. Only the hash of the cookie value is stored.
type Session struct {
	ID        ids.ID
	ActorID   ids.ID
	TokenHash []byte
	CSRFToken string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStat is one actor's login activity.
type SessionStat struct {
	Active    int
	LastLogin *time.Time
}

// SessionTTL is how long a login lasts.
const SessionTTL = 30 * 24 * time.Hour

// Login errors.
var (
	ErrBadCredentials    = errors.New("wrong handle or password")
	ErrTooManyAttempts   = errors.New("too many failed logins; try again later")
	ErrWorkspaceRequired = errors.New("this handle exists in several workspaces; give the workspace name")
)

// CSRFMatches compares a submitted CSRF token in constant time.
func (s Session) CSRFMatches(token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.CSRFToken)) == 1
}

// LoginInput is a password login attempt.
type LoginInput struct {
	Handle    string
	Password  string
	Workspace string // only needed when the handle exists in several workspaces
	UserAgent string
}

// Login checks a password and opens a session. It returns the actor, the
// session and the cookie secret (shown once).
func (s *Service) Login(ctx context.Context, in LoginInput) (Actor, Session, string, error) {
	if !s.limiter().allow(in.Handle, s.Clock.Now()) {
		return Actor{}, Session{}, "", ErrTooManyAttempts
	}
	fail := func() (Actor, Session, string, error) {
		s.limiter().fail(in.Handle, s.Clock.Now())
		return Actor{}, Session{}, "", ErrBadCredentials
	}
	a, err := s.findLoginActor(ctx, in)
	if errors.Is(err, ErrWorkspaceRequired) {
		return Actor{}, Session{}, "", err
	}
	if err != nil || a.Kind != KindHuman || a.State != StateActive {
		_, _ = VerifyPassword(in.Password, dummyHash) // equalise timing
		return fail()
	}
	hash, err := s.Store.Credential(ctx, a.ID)
	if errors.Is(err, db.ErrNotFound) {
		_, _ = VerifyPassword(in.Password, dummyHash)
		return fail()
	}
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	ok, err := VerifyPassword(in.Password, hash)
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	if !ok {
		s.recordLogin(ctx, a, EventLoginFailed, in.UserAgent)
		return fail()
	}
	s.limiter().reset(in.Handle)

	secret, err := NewSecret()
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	csrf, err := NewSecret()
	if err != nil {
		return Actor{}, Session{}, "", err
	}
	now := s.Clock.Now()
	sess := Session{ID: s.IDs.New(), ActorID: a.ID, TokenHash: HashSecret(secret), CSRFToken: csrf,
		UserAgent: truncate(in.UserAgent, 256), CreatedAt: now, ExpiresAt: now.Add(SessionTTL)}
	if err := s.Store.CreateSession(ctx, sess); err != nil {
		return Actor{}, Session{}, "", err
	}
	s.recordLogin(ctx, a, EventLogin, in.UserAgent)
	return a, sess, secret, nil
}

// recordLogin writes a login event for the audit log. A failure to record
// is ignored on purpose: it must never lock people out.
func (s *Service) recordLogin(ctx context.Context, a Actor, typ, userAgent string) {
	_ = s.record(ctx, a, typ, "actor", a.ID, LoginEvent{ActorID: a.ID.String(), UserAgent: truncate(userAgent, 256)},
		func(context.Context) error { return nil })
}

func (s *Service) findLoginActor(ctx context.Context, in LoginInput) (Actor, error) {
	matches, err := s.Store.ActorsByHandle(ctx, in.Handle)
	if err != nil {
		return Actor{}, err
	}
	if in.Workspace != "" {
		ws, err := s.Store.WorkspaceByName(ctx, in.Workspace)
		if err != nil {
			return Actor{}, err
		}
		for _, a := range matches {
			if a.WorkspaceID == ws.ID {
				return a, nil
			}
		}
		return Actor{}, db.ErrNotFound
	}
	switch len(matches) {
	case 0:
		return Actor{}, db.ErrNotFound
	case 1:
		return matches[0], nil
	default:
		return Actor{}, ErrWorkspaceRequired
	}
}

// FindHuman finds a person by handle (and workspace name when ambiguous).
func (s *Service) FindHuman(ctx context.Context, handle, workspace string) (Actor, error) {
	a, err := s.findLoginActor(ctx, LoginInput{Handle: handle, Workspace: workspace})
	if err != nil {
		return Actor{}, err
	}
	if a.Kind != KindHuman {
		return Actor{}, ErrForbidden
	}
	return a, nil
}

// AuthenticateSession resolves a session cookie to an active actor.
func (s *Service) AuthenticateSession(ctx context.Context, secret string) (Actor, Session, error) {
	if secret == "" {
		return Actor{}, Session{}, ErrUnauthorized
	}
	sess, a, err := s.Store.SessionByTokenHash(ctx, HashSecret(secret))
	if errors.Is(err, db.ErrNotFound) {
		return Actor{}, Session{}, ErrUnauthorized
	}
	if err != nil {
		return Actor{}, Session{}, err
	}
	if !s.Clock.Now().Before(sess.ExpiresAt) {
		return Actor{}, Session{}, ErrUnauthorized
	}
	a, err = s.load(ctx, a)
	if err != nil {
		return Actor{}, Session{}, err
	}
	return a, sess, nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, sessionID ids.ID) error {
	return s.Store.RevokeSession(ctx, sessionID, s.Clock.Now())
}

// SetPassword changes a human's password. People may change their own
// (with the current password; their current session stays logged in).
// Human admins may set the password of lower-ranked people only. Every
// other session of the target is revoked.
func (s *Service) SetPassword(ctx context.Context, by Actor, currentSession, targetID ids.ID, current, next string) error {
	target, err := s.Store.ActorByID(ctx, targetID)
	if err != nil {
		return err
	}
	if target.WorkspaceID != by.WorkspaceID {
		return db.ErrNotFound
	}
	if target.Kind != KindHuman {
		return ErrForbidden
	}
	switch {
	case by.ID == target.ID:
		hash, err := s.Store.Credential(ctx, target.ID)
		if err == nil {
			ok, verr := VerifyPassword(current, hash)
			if verr != nil {
				return verr
			}
			if !ok {
				return ErrBadCredentials
			}
		} else if !errors.Is(err, db.ErrNotFound) {
			return err
		}
	case by.CanManage(target) && by.Role == RoleOwner:
		// Only the owner resets another person's password.
		if err := ValidatePassword(next); err != nil {
			return err
		}
		if err := s.record(ctx, by, EventPasswordReset, "actor", target.ID, PasswordReset{ActorID: target.ID.String()},
			func(ctx context.Context) error { return s.ResetPassword(ctx, target.ID, next) }); err != nil {
			return err
		}
		return s.Store.RevokeSessionsForActor(ctx, target.ID, ids.Nil, s.Clock.Now())
	default:
		return ErrForbidden
	}
	if err := s.ResetPassword(ctx, target.ID, next); err != nil {
		return err
	}
	// A changed password logs out every other browser of that person.
	return s.Store.RevokeSessionsForActor(ctx, target.ID, currentSession, s.Clock.Now())
}

// ResetPassword sets a password without any permission check. Only trusted
// callers (the command line, the development seed) use it.
func (s *Service) ResetPassword(ctx context.Context, actorID ids.ID, password string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.Store.SetCredential(ctx, actorID, hash, s.Clock.Now())
}

// loginLimiter blocks a handle after repeated failures. It is per process;
// a shared limiter arrives with the rate_limit_event table.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

const (
	maxLoginFailures = 5
	loginWindow      = 15 * time.Minute
)

func (s *Service) limiter() *loginLimiter {
	s.limiterOnce.Do(func() { s.loginLimiter = &loginLimiter{failures: map[string][]time.Time{}} })
	return s.loginLimiter
}

func (l *loginLimiter) recent(handle string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range l.failures[handle] {
		if now.Sub(t) < loginWindow {
			keep = append(keep, t)
		}
	}
	l.failures[handle] = keep
	return keep
}

func (l *loginLimiter) allow(handle string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(handle, now)) < maxLoginFailures
}

func (l *loginLimiter) fail(handle string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[handle] = append(l.recent(handle, now), now)
}

func (l *loginLimiter) reset(handle string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, handle)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
