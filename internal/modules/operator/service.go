// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package operator

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/db/opscope"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// Limiter is the durable login limit of round 40 (identity.Service).
type Limiter interface {
	CheckLimit(ctx context.Context, scope, handle string) error
	LimitFailed(ctx context.Context, scope, handle string, workspaceID ids.ID)
	LimitSucceeded(ctx context.Context, scope, handle string)
}

// Service holds the operator use cases.
type Service struct {
	Store     Store
	Tx        db.TxRunner
	Keys      *secretbox.Keyring
	Limiter   Limiter
	Customers Customers
	// BreakGlass opens and closes the read-only accesses (round 51b).
	BreakGlass BreakGlassAccess
	Clock      clock.Clock
	IDs        ids.Generator
	// DevAccess: COLONY_OPERATOR_DEV_ACCESS=1 (development only); every
	// operator page shows it.
	DevAccess bool
}

// scope puts ctx in the operator database scope.
func scope(ctx context.Context) context.Context { return opscope.Context(ctx) }

// Enrollment is a new TOTP secret, shown once.
type Enrollment struct {
	Secret string
	URI    string
}

func (s *Service) seal(id ids.ID, secret string) (secretbox.Sealed, error) {
	if s.Keys == nil {
		return secretbox.Sealed{}, ErrNoKeys
	}
	return s.Keys.Seal([]byte(secret), totpAAD(id))
}

func (s *Service) open(o Operator) (string, error) {
	if s.Keys == nil {
		return "", ErrNoKeys
	}
	b, err := s.Keys.Open(o.TOTP, totpAAD(o.ID))
	return string(b), err
}

// CreateOperator creates a pending operator (the CLI). The account becomes
// active once ConfirmTOTP accepts the first code.
func (s *Service) CreateOperator(ctx context.Context, handle, displayName, password string) (Operator, Enrollment, error) {
	ctx = scope(ctx)
	handle = strings.ToLower(strings.TrimSpace(handle))
	if !ValidHandle(handle) {
		return Operator{}, Enrollment{}, ErrInvalidHandle
	}
	if err := identity.ValidatePassword(password); err != nil {
		return Operator{}, Enrollment{}, err
	}
	if displayName = strings.TrimSpace(displayName); displayName == "" {
		displayName = handle
	}
	hash, err := identity.HashPassword(password)
	if err != nil {
		return Operator{}, Enrollment{}, err
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return Operator{}, Enrollment{}, err
	}
	o := Operator{ID: s.IDs.New(), Handle: handle, DisplayName: displayName, PasswordHash: hash, State: StatePending, CreatedAt: s.Clock.Now()}
	if o.TOTP, err = s.seal(o.ID, secret); err != nil {
		return Operator{}, Enrollment{}, err
	}
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateOperator(ctx, o); err != nil {
			if errors.Is(err, db.ErrConflict) {
				return ErrExists
			}
			return err
		}
		return s.audit(ctx, ids.Nil, ActionCreated, ids.Nil, map[string]any{"operator_id": o.ID.String(), "handle": handle}, "")
	})
	if err != nil {
		return Operator{}, Enrollment{}, err
	}
	return o, Enrollment{Secret: secret, URI: totpURI(handle, secret)}, nil
}

// ConfirmTOTP accepts the first code of a pending operator, activates the
// account and returns its backup codes (shown once).
func (s *Service) ConfirmTOTP(ctx context.Context, handle, code string) ([]string, error) {
	ctx = scope(ctx)
	o, err := s.Store.OperatorByHandle(ctx, strings.ToLower(strings.TrimSpace(handle)))
	if err != nil {
		return nil, err
	}
	if o.State != StatePending {
		return nil, ErrNotPending
	}
	secret, err := s.open(o)
	if err != nil {
		return nil, err
	}
	step, ok := totpMatch(secret, strings.TrimSpace(code), s.Clock.Now(), o.TOTPLastStep)
	if !ok {
		return nil, ErrBadCode
	}
	var codes []string
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if ok, err := s.Store.AdvanceTOTPStep(ctx, o.ID, step); err != nil || !ok {
			return errors.Join(err, ErrBadCode)
		}
		now := s.Clock.Now()
		if err := s.Store.SetState(ctx, o.ID, StateActive, &now); err != nil {
			return err
		}
		var err error
		if codes, err = s.replaceBackupCodes(ctx, o.ID); err != nil {
			return err
		}
		return s.audit(ctx, ids.Nil, ActionActivated, ids.Nil, map[string]any{"operator_id": o.ID.String(), "handle": o.Handle}, "")
	})
	return codes, err
}

// ResetTOTP gives an operator a new TOTP secret (the CLI, e.g. after a
// lost phone): the account is pending until ConfirmTOTP, its sessions end
// and its backup codes are void.
func (s *Service) ResetTOTP(ctx context.Context, handle string) (Enrollment, error) {
	ctx = scope(ctx)
	o, err := s.Store.OperatorByHandle(ctx, strings.ToLower(strings.TrimSpace(handle)))
	if err != nil {
		return Enrollment{}, err
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return Enrollment{}, err
	}
	sealed, err := s.seal(o.ID, secret)
	if err != nil {
		return Enrollment{}, err
	}
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		now := s.Clock.Now()
		if err := s.Store.SetTOTP(ctx, o.ID, sealed, StatePending); err != nil {
			return err
		}
		if err := s.Store.ReplaceBackupCodes(ctx, o.ID, nil, now); err != nil {
			return err
		}
		if err := s.Store.RevokeSessions(ctx, o.ID, now); err != nil {
			return err
		}
		return s.audit(ctx, ids.Nil, ActionTOTPReset, ids.Nil, map[string]any{"operator_id": o.ID.String(), "handle": o.Handle}, "")
	})
	if err != nil {
		return Enrollment{}, err
	}
	return Enrollment{Secret: secret, URI: totpURI(o.Handle, secret)}, nil
}

// Disable turns an operator off and ends their sessions (the CLI).
func (s *Service) Disable(ctx context.Context, handle string) error {
	ctx = scope(ctx)
	o, err := s.Store.OperatorByHandle(ctx, strings.ToLower(strings.TrimSpace(handle)))
	if err != nil {
		return err
	}
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.SetState(ctx, o.ID, StateDisabled, o.ActivatedAt); err != nil {
			return err
		}
		if err := s.Store.RevokeSessions(ctx, o.ID, s.Clock.Now()); err != nil {
			return err
		}
		return s.audit(ctx, ids.Nil, ActionDisabled, ids.Nil, map[string]any{"operator_id": o.ID.String(), "handle": o.Handle}, "")
	})
}

// RotateSecrets re-encrypts every TOTP secret sealed with an older key
// with the active one; it returns how many it changed.
func (s *Service) RotateSecrets(ctx context.Context) (int, error) {
	ctx = scope(ctx)
	if s.Keys == nil {
		return 0, ErrNoKeys
	}
	list, err := s.Store.ListOperators(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range list {
		if !s.Keys.NeedsRotation(o.TOTP) {
			continue
		}
		secret, err := s.open(o)
		if err != nil {
			return n, fmt.Errorf("operator %s: %w", o.Handle, err)
		}
		sealed, err := s.seal(o.ID, secret)
		if err != nil {
			return n, err
		}
		if err := s.Store.ResealTOTP(ctx, o.ID, sealed); err != nil {
			return n, err
		}
		n++
	}
	if n > 0 {
		err = s.Tx.InTx(ctx, func(ctx context.Context) error {
			return s.audit(ctx, ids.Nil, ActionSecretsRotated, ids.Nil, map[string]any{"count": n, "key_id": s.Keys.Active()}, "")
		})
	}
	return n, err
}

// LoginInput is a login attempt: the password and a TOTP or backup code.
type LoginInput struct {
	Handle     string
	Password   string
	Code       string
	RemoteAddr string
	UserAgent  string
}

// dummyHash equalises the timing of a login with an unknown handle.
var dummyHash, _ = identity.HashPassword("colonyhall-operator-dummy-password")

// Login checks the password and the code and opens a session. Every
// failure is the same ErrBadCredentials, counted by the login limit.
func (s *Service) Login(ctx context.Context, in LoginInput) (Operator, Session, string, error) {
	ctx = scope(ctx)
	handle := strings.ToLower(strings.TrimSpace(in.Handle))
	if err := s.Limiter.CheckLimit(ctx, identity.LimitOperatorLogin, handle); err != nil {
		return Operator{}, Session{}, "", err
	}
	o, err := s.Store.OperatorByHandle(ctx, handle)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return Operator{}, Session{}, "", err
	}
	fail := func(reason string) (Operator, Session, string, error) {
		s.Limiter.LimitFailed(ctx, identity.LimitOperatorLogin, handle, ids.Nil)
		_ = s.Tx.InTx(ctx, func(ctx context.Context) error {
			return s.audit(ctx, o.ID, ActionLoginFailed, ids.Nil, map[string]any{"handle": truncate(handle, 64), "reason": reason}, in.RemoteAddr)
		})
		return Operator{}, Session{}, "", ErrBadCredentials
	}
	if err != nil || o.State != StateActive {
		_, _ = identity.VerifyPassword(in.Password, dummyHash)
		if err == nil {
			return fail("not_active")
		}
		o = Operator{}
		return fail("unknown_handle")
	}
	ok, err := identity.VerifyPassword(in.Password, o.PasswordHash)
	if err != nil {
		return Operator{}, Session{}, "", err
	}
	if !ok {
		return fail("password")
	}
	usedBackup := false
	code := strings.TrimSpace(in.Code)
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if isTOTPCode(code) {
			secret, err := s.open(o)
			if err != nil {
				return err
			}
			step, ok := totpMatch(secret, code, s.Clock.Now(), o.TOTPLastStep)
			if !ok {
				return ErrBadCode
			}
			if ok, err := s.Store.AdvanceTOTPStep(ctx, o.ID, step); err != nil || !ok {
				return errors.Join(err, ErrBadCode)
			}
			return nil
		}
		ok, err := s.Store.UseBackupCode(ctx, o.ID, hashBackupCode(code), s.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return ErrBadCode
		}
		usedBackup = true
		return nil
	})
	if errors.Is(err, ErrBadCode) {
		return fail("code")
	}
	if err != nil {
		return Operator{}, Session{}, "", err
	}
	s.Limiter.LimitSucceeded(ctx, identity.LimitOperatorLogin, handle)

	secret, err := identity.NewSecret()
	if err != nil {
		return Operator{}, Session{}, "", err
	}
	csrf, err := identity.NewSecret()
	if err != nil {
		return Operator{}, Session{}, "", err
	}
	now := s.Clock.Now()
	sess := Session{ID: s.IDs.New(), OperatorID: o.ID, TokenHash: identity.HashSecret(secret), CSRFToken: csrf,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(SessionTTL), RemoteAddr: truncate(in.RemoteAddr, 64), UserAgent: truncate(in.UserAgent, 256)}
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.CreateSession(ctx, sess); err != nil {
			return err
		}
		if err := s.Store.SetLastLogin(ctx, o.ID, now); err != nil {
			return err
		}
		if usedBackup {
			left, err := s.Store.UnusedBackupCodes(ctx, o.ID)
			if err != nil {
				return err
			}
			if err := s.audit(ctx, o.ID, ActionBackupCodeUsed, ids.Nil, map[string]any{"left": left}, in.RemoteAddr); err != nil {
				return err
			}
		}
		return s.audit(ctx, o.ID, ActionLogin, ids.Nil, map[string]any{"session_id": sess.ID.String()}, in.RemoteAddr)
	})
	if err != nil {
		return Operator{}, Session{}, "", err
	}
	o.LastLoginAt = &now
	return o, sess, secret, nil
}

// Authenticate checks a session cookie: live (8 hours at most, 30 minutes
// idle), not revoked, and its operator active.
func (s *Service) Authenticate(ctx context.Context, secret string) (Operator, Session, error) {
	ctx = scope(ctx)
	if secret == "" {
		return Operator{}, Session{}, ErrUnauthorized
	}
	sess, err := s.Store.SessionByTokenHash(ctx, identity.HashSecret(secret))
	if errors.Is(err, db.ErrNotFound) {
		return Operator{}, Session{}, ErrUnauthorized
	}
	if err != nil {
		return Operator{}, Session{}, err
	}
	now := s.Clock.Now()
	if subtle.ConstantTimeCompare(sess.TokenHash, identity.HashSecret(secret)) != 1 || !sess.Live(now) {
		return Operator{}, Session{}, ErrUnauthorized
	}
	o, err := s.Store.OperatorByID(ctx, sess.OperatorID)
	if err != nil || o.State != StateActive {
		return Operator{}, Session{}, ErrUnauthorized
	}
	if now.Sub(sess.LastSeenAt) >= touchEvery {
		if err := s.Store.TouchSession(ctx, sess.ID, now); err != nil {
			return Operator{}, Session{}, err
		}
		sess.LastSeenAt = now
	}
	return o, sess, nil
}

// Logout ends a session.
func (s *Service) Logout(ctx context.Context, o Operator, sess Session, remote string) error {
	ctx = scope(ctx)
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.Store.RevokeSession(ctx, sess.ID, s.Clock.Now()); err != nil {
			return err
		}
		return s.audit(ctx, o.ID, ActionLogout, ids.Nil, map[string]any{"session_id": sess.ID.String()}, remote)
	})
}

// BackupCodesLeft counts the operator's unused backup codes.
func (s *Service) BackupCodesLeft(ctx context.Context, o Operator) (int, error) {
	return s.Store.UnusedBackupCodes(scope(ctx), o.ID)
}

// RenewBackupCodes replaces the operator's backup codes, after checking the
// password and a TOTP code again. The new codes are shown once.
func (s *Service) RenewBackupCodes(ctx context.Context, o Operator, password, code, remote string) ([]string, error) {
	ctx = scope(ctx)
	if err := s.Limiter.CheckLimit(ctx, identity.LimitOperatorLogin, o.Handle); err != nil {
		return nil, err
	}
	o, err := s.Store.OperatorByID(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	fail := func() ([]string, error) {
		s.Limiter.LimitFailed(ctx, identity.LimitOperatorLogin, o.Handle, ids.Nil)
		return nil, ErrBadCredentials
	}
	if ok, err := identity.VerifyPassword(password, o.PasswordHash); err != nil {
		return nil, err
	} else if !ok {
		return fail()
	}
	secret, err := s.open(o)
	if err != nil {
		return nil, err
	}
	step, ok := totpMatch(secret, strings.TrimSpace(code), s.Clock.Now(), o.TOTPLastStep)
	if !ok {
		return fail()
	}
	var codes []string
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		if ok, err := s.Store.AdvanceTOTPStep(ctx, o.ID, step); err != nil || !ok {
			return errors.Join(err, ErrBadCredentials)
		}
		var err error
		if codes, err = s.replaceBackupCodes(ctx, o.ID); err != nil {
			return err
		}
		return s.audit(ctx, o.ID, ActionBackupCodesRenewed, ids.Nil, nil, remote)
	})
	if errors.Is(err, ErrBadCredentials) {
		return fail()
	}
	return codes, err
}

func (s *Service) replaceBackupCodes(ctx context.Context, operatorID ids.ID) ([]string, error) {
	codes := make([]string, 0, BackupCodeCount)
	stored := make([]BackupCode, 0, BackupCodeCount)
	for range BackupCodeCount {
		c, err := newBackupCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, c)
		stored = append(stored, BackupCode{ID: s.IDs.New(), Hash: hashBackupCode(c)})
	}
	return codes, s.Store.ReplaceBackupCodes(ctx, operatorID, stored, s.Clock.Now())
}

// audit appends an operator audit entry (inside the caller's transaction).
func (s *Service) audit(ctx context.Context, operatorID ids.ID, action string, workspaceID ids.ID, payload map[string]any, remote string) error {
	if payload == nil {
		payload = map[string]any{}
	}
	return s.Store.AppendAudit(ctx, AuditEntry{ID: s.IDs.New(), OperatorID: operatorID, Action: action, WorkspaceID: workspaceID,
		Payload: payload, RemoteAddr: truncate(remote, 64), CreatedAt: s.Clock.Now()})
}

// record appends an audit entry in its own transaction.
func (s *Service) record(ctx context.Context, o Operator, action string, workspaceID ids.ID, payload map[string]any, remote string) error {
	return s.Tx.InTx(scope(ctx), func(ctx context.Context) error {
		return s.audit(ctx, o.ID, action, workspaceID, payload, remote)
	})
}

// Audit lists the operator audit, newest first.
func (s *Service) Audit(ctx context.Context, before time.Time, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.Store.ListAudit(scope(ctx), before, limit)
}

// CleanupSessions deletes sessions that ended more than a day ago.
func (s *Service) CleanupSessions(ctx context.Context) (int64, error) {
	return s.Store.DeleteEndedSessions(scope(ctx), s.Clock.Now().Add(-24*time.Hour))
}

// Customers (the workspaces), through the Customers port. Each change is
// recorded in the operator audit; the workspace's own audit gets its event
// from the identity module.

// ListCustomers lists every workspace without content.
// Every Customers call runs in the operator scope: the reads stay in it
// (Postgres refuses the overview anywhere else); the writes narrow to the
// one workspace they change.
func (s *Service) ListCustomers(ctx context.Context) ([]Customer, error) {
	return s.Customers.List(scope(ctx))
}

// Customer returns one workspace without content.
func (s *Service) Customer(ctx context.Context, id ids.ID) (Customer, error) {
	return s.Customers.Get(scope(ctx), id)
}

// CreateCustomer creates a workspace and its owner invite.
func (s *Service) CreateCustomer(ctx context.Context, o Operator, name, slug, remote string) (Customer, InviteLink, error) {
	ctx = scope(ctx)
	c, err := s.Customers.Create(ctx, name, slug)
	if err != nil {
		return Customer{}, InviteLink{}, err
	}
	if err := s.record(ctx, o, ActionWorkspaceCreated, c.ID, map[string]any{"name": c.Name, "slug": c.Slug}, remote); err != nil {
		return Customer{}, InviteLink{}, err
	}
	link, err := s.Invite(ctx, o, c.ID, remote)
	if err != nil {
		return Customer{}, InviteLink{}, err
	}
	c, err = s.Customers.Get(ctx, c.ID)
	return c, link, err
}

// Invite makes a new owner invite (the old one is revoked).
func (s *Service) Invite(ctx context.Context, o Operator, id ids.ID, remote string) (InviteLink, error) {
	link, err := s.Customers.Invite(scope(ctx), id, o.ID)
	if err != nil {
		return InviteLink{}, err
	}
	return link, s.record(ctx, o, ActionInviteIssued, id, map[string]any{"expires_at": link.ExpiresAt}, remote)
}

// SetSuspended suspends (with a reason) or restores a workspace.
func (s *Service) SetSuspended(ctx context.Context, o Operator, id ids.ID, suspend bool, reason, remote string) (Customer, error) {
	c, err := s.Customers.SetSuspended(scope(ctx), id, suspend, reason)
	if err != nil {
		return Customer{}, err
	}
	action, payload := ActionWorkspaceRestored, map[string]any{}
	if suspend {
		action, payload = ActionWorkspaceSuspended, map[string]any{"reason": reason}
	}
	return c, s.record(ctx, o, action, id, payload, remote)
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
