// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package identity

import (
	"context"
	"crypto/subtle"
	"errors"
	"slices"
	"time"

	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/tenant"
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
	// LastSeenAt is when it last authenticated a request (a minute apart at most).
	LastSeenAt *time.Time
}

// SessionStat is one actor's login activity.
type SessionStat struct {
	Active    int
	LastLogin *time.Time
	// LastSeen is the latest use of any of the person's sessions.
	LastSeen *time.Time
}

// touchEvery throttles the last-use writes of tokens and sessions.
const touchEvery = time.Minute

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
	if err := s.CheckLimit(ctx, LimitLogin, in.Handle); err != nil {
		return Actor{}, Session{}, "", err
	}
	ctx, a, err := s.findLoginActor(ctx, in)
	if errors.Is(err, ErrWorkspaceRequired) {
		return Actor{}, Session{}, "", err
	}
	// The failure belongs to the handle's workspace when there is one, or
	// to the subdomain's (multi-tenant mode).
	failWS := a.WorkspaceID
	if t, ok := tenant.From(ctx); ok && failWS == ids.Nil {
		failWS = t.Workspace
	}
	fail := func() (Actor, Session, string, error) {
		s.LimitFailed(ctx, LimitLogin, in.Handle, failWS)
		return Actor{}, Session{}, "", ErrBadCredentials
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
	s.LimitSucceeded(ctx, LimitLogin, in.Handle)

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

// findLoginActor finds the person logging in and returns ctx narrowed to
// their workspace (round 45: the handle is looked up across workspaces
// first, then read inside the one it belongs to).
func (s *Service) findLoginActor(ctx context.Context, in LoginInput) (context.Context, Actor, error) {
	wss, err := s.Store.HandleWorkspaces(ctx, in.Handle)
	if err != nil {
		return ctx, Actor{}, err
	}
	if t, ok := tenant.From(ctx); ok {
		// Multi-tenant mode: only the subdomain's workspace; a handle that
		// exists elsewhere is simply not found here.
		wss = slices.DeleteFunc(wss, func(w Workspace) bool { return w.ID != t.Workspace })
	} else if in.Workspace != "" {
		wss = slices.DeleteFunc(wss, func(w Workspace) bool { return w.Name != in.Workspace })
	}
	switch len(wss) {
	case 0:
		return ctx, Actor{}, db.ErrNotFound
	case 1:
	default:
		return ctx, Actor{}, ErrWorkspaceRequired
	}
	ctx = db.WithWorkspace(ctx, wss[0].ID)
	a, err := s.Store.ActorByHandle(ctx, wss[0].ID, in.Handle)
	return ctx, a, err
}

// FindHuman finds a person by handle (and workspace name when ambiguous).
func (s *Service) FindHuman(ctx context.Context, handle, workspace string) (Actor, error) {
	_, a, err := s.findLoginActor(ctx, LoginInput{Handle: handle, Workspace: workspace})
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
	ctx, err := s.credentialScope(ctx, credSession, s.Store.WorkspaceOfSession, HashSecret(secret))
	if err != nil {
		return Actor{}, Session{}, err
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
	now := s.Clock.Now()
	_ = s.Store.TouchSession(ctx, sess.ID, now, now.Add(-touchEvery))
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
		// Limited apart from the login (round 40): a stolen session must
		// not be able to guess the current password without bound.
		if err := s.CheckLimit(ctx, LimitPassword, by.Handle); err != nil {
			return err
		}
		hash, err := s.Store.Credential(ctx, target.ID)
		if err == nil {
			ok, verr := VerifyPassword(current, hash)
			if verr != nil {
				return verr
			}
			if !ok {
				s.LimitFailed(ctx, LimitPassword, by.Handle, by.WorkspaceID)
				return ErrBadCredentials
			}
			s.LimitSucceeded(ctx, LimitPassword, by.Handle)
		} else if !errors.Is(err, db.ErrNotFound) {
			return err
		}
	case by.CanManage(target) && by.Role == RoleOwner:
		// Only the owner resets another person's password.
		if err := ValidatePassword(next); err != nil {
			return err
		}
		if err := s.record(ctx, by, EventPasswordReset, "actor", target.ID, PasswordReset{ActorID: target.ID.String()},
			func(ctx context.Context) error {
				if err := s.ResetPassword(ctx, target.ID, next); err != nil {
					return err
				}
				return s.Store.RevokeSessionsForActor(ctx, target.ID, ids.Nil, s.Clock.Now())
			}); err != nil {
			return err
		}
		return nil
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
