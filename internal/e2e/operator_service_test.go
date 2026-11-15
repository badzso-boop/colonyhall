// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/operator"
	"colonyhall/internal/modules/operator/memory"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// fakeLimiter counts failures per handle; locks at 5.
type fakeLimiter struct{ failed map[string]int }

func (l *fakeLimiter) CheckLimit(_ context.Context, _, handle string) error {
	if l.failed[handle] >= 5 {
		return identity.ErrTooManyAttempts
	}
	return nil
}
func (l *fakeLimiter) LimitFailed(_ context.Context, _, handle string, _ ids.ID) { l.failed[handle]++ }
func (l *fakeLimiter) LimitSucceeded(_ context.Context, _, handle string)        { l.failed[handle] = 0 }

const opPassword = "correct horse battery staple"

type opHarness struct {
	svc   *operator.Service
	store *memory.Store
	clock *clock.Manual
	lim   *fakeLimiter
	keys  *secretbox.Keyring
	key   string
}

func newOpHarness(t *testing.T) *opHarness {
	t.Helper()
	k, _ := secretbox.NewKey("k1")
	keys, err := secretbox.Parse(k)
	if err != nil {
		t.Fatal(err)
	}
	h := &opHarness{store: memory.New(), clock: clock.NewManual(time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)),
		lim: &fakeLimiter{failed: map[string]int{}}, keys: keys, key: k}
	h.svc = &operator.Service{Store: h.store, Tx: &db.Memory{}, Keys: keys, Limiter: h.lim, Clock: h.clock, IDs: &ids.Sequential{}}
	return h
}

// enroll creates an active operator and returns its TOTP secret and
// backup codes.
func (h *opHarness) enroll(t *testing.T, handle string) (string, []string) {
	t.Helper()
	ctx := context.Background()
	_, en, err := h.svc.CreateOperator(ctx, handle, "", opPassword)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(en.URI, "otpauth://totp/") || !strings.Contains(en.URI, "secret="+en.Secret) {
		t.Fatalf("uri: %s", en.URI)
	}
	// A pending operator cannot log in.
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: handle, Password: opPassword, Code: operator.CodeAt(en.Secret, h.clock.Now())}); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("a pending operator logged in: %v", err)
	}
	h.lim.failed[handle] = 0
	if _, err := h.svc.ConfirmTOTP(ctx, handle, "000000"); !errors.Is(err, operator.ErrBadCode) {
		t.Fatalf("a wrong first code: %v", err)
	}
	codes, err := h.svc.ConfirmTOTP(ctx, handle, operator.CodeAt(en.Secret, h.clock.Now()))
	if err != nil || len(codes) != operator.BackupCodeCount {
		t.Fatalf("confirm: %v %v", codes, err)
	}
	h.clock.Advance(30 * time.Second) // the confirming code is used up
	return en.Secret, codes
}

func TestOperatorLoginNeedsPasswordAndCode(t *testing.T) {
	h := newOpHarness(t)
	ctx := context.Background()
	secret, codes := h.enroll(t, "ops")
	code := func() string { return operator.CodeAt(secret, h.clock.Now()) }

	for name, in := range map[string]operator.LoginInput{
		"wrong password": {Handle: "ops", Password: "wrong password here", Code: code()},
		"wrong code":     {Handle: "ops", Password: opPassword, Code: "123456"},
		"no code":        {Handle: "ops", Password: opPassword},
		"unknown handle": {Handle: "nobody", Password: opPassword, Code: code()},
	} {
		if _, _, _, err := h.svc.Login(ctx, in); !errors.Is(err, operator.ErrBadCredentials) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	o, sess, cookie, err := h.svc.Login(ctx, operator.LoginInput{Handle: "OPS ", Password: opPassword, Code: code(), RemoteAddr: "192.0.2.1"})
	if err != nil || o.Handle != "ops" || cookie == "" || sess.CSRFToken == "" {
		t.Fatalf("login: %+v %v", o, err)
	}
	// The same code again (a replay within its step) is refused.
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: code()}); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("a replayed code: %v", err)
	}
	// A backup code works once, in any case and with or without the dash.
	h.clock.Advance(time.Minute)
	bc := strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: bc}); err != nil {
		t.Fatalf("backup code: %v", err)
	}
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: codes[0]}); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("a used backup code: %v", err)
	}
	if left, _ := h.svc.BackupCodesLeft(ctx, o); left != operator.BackupCodeCount-1 {
		t.Fatalf("left: %d", left)
	}
	// Failures are counted; at the limit even the right answer is refused.
	h.lim.failed["ops"] = 5
	h.clock.Advance(time.Minute)
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: code()}); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("locked out: %v", err)
	}
	// Every attempt is in the operator audit, never the password or code.
	entries, err := h.svc.Audit(ctx, time.Time{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	var logins, failed int
	for _, e := range entries {
		for _, v := range e.Payload {
			if s, ok := v.(string); ok && (s == opPassword || s == code()) {
				t.Fatalf("a secret in the audit: %+v", e)
			}
		}
		switch e.Action {
		case operator.ActionLogin:
			logins++
		case operator.ActionLoginFailed:
			failed++
		}
	}
	if logins != 2 || failed < 6 {
		t.Fatalf("audit: %d logins, %d failures", logins, failed)
	}
}

func TestOperatorSessionLimits(t *testing.T) {
	h := newOpHarness(t)
	ctx := context.Background()
	secret, _ := h.enroll(t, "ops")
	login := func() string {
		t.Helper()
		h.clock.Advance(31 * time.Second)
		_, _, cookie, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: operator.CodeAt(secret, h.clock.Now())})
		if err != nil {
			t.Fatal(err)
		}
		return cookie
	}

	// Idle: 30 minutes without a request ends it.
	cookie := login()
	h.clock.Advance(29 * time.Minute)
	if _, _, err := h.svc.Authenticate(ctx, cookie); err != nil {
		t.Fatalf("active session: %v", err)
	}
	h.clock.Advance(31 * time.Minute)
	if _, _, err := h.svc.Authenticate(ctx, cookie); !errors.Is(err, operator.ErrUnauthorized) {
		t.Fatalf("idle session: %v", err)
	}

	// Absolute: 8 hours even when used all along.
	cookie = login()
	for range 16 {
		h.clock.Advance(29 * time.Minute)
		if _, _, err := h.svc.Authenticate(ctx, cookie); err != nil {
			t.Fatalf("used session before 8 hours: %v", err)
		}
	}
	h.clock.Advance(29 * time.Minute)
	if _, _, err := h.svc.Authenticate(ctx, cookie); !errors.Is(err, operator.ErrUnauthorized) {
		t.Fatalf("a session past 8 hours: %v", err)
	}

	// Logout and disabling end it.
	cookie = login()
	o, sess, err := h.svc.Authenticate(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.Logout(ctx, o, sess, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.svc.Authenticate(ctx, cookie); !errors.Is(err, operator.ErrUnauthorized) {
		t.Fatalf("after logout: %v", err)
	}
	cookie = login()
	if err := h.svc.Disable(ctx, "ops"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.svc.Authenticate(ctx, cookie); !errors.Is(err, operator.ErrUnauthorized) {
		t.Fatalf("after disable: %v", err)
	}
	if _, _, err := h.svc.Authenticate(ctx, "garbage"); !errors.Is(err, operator.ErrUnauthorized) {
		t.Fatalf("garbage cookie: %v", err)
	}
}

func TestOperatorResetTOTPAndRenewBackupCodes(t *testing.T) {
	h := newOpHarness(t)
	ctx := context.Background()
	secret, codes := h.enroll(t, "ops")
	_, _, cookie, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: operator.CodeAt(secret, h.clock.Now())})
	if err != nil {
		t.Fatal(err)
	}
	o, _, _ := h.svc.Authenticate(ctx, cookie)

	// Renewing needs the password and a fresh code; the old codes stop.
	h.clock.Advance(time.Minute)
	if _, err := h.svc.RenewBackupCodes(ctx, o, "wrong password here", operator.CodeAt(secret, h.clock.Now()), ""); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("renew with a wrong password: %v", err)
	}
	fresh, err := h.svc.RenewBackupCodes(ctx, o, opPassword, operator.CodeAt(secret, h.clock.Now()), "")
	if err != nil || len(fresh) != operator.BackupCodeCount {
		t.Fatalf("renew: %v %v", fresh, err)
	}
	h.clock.Advance(time.Minute)
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: codes[1]}); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("an old backup code after renewal: %v", err)
	}

	// A reset: the old secret, the backup codes and the sessions stop; the
	// new secret works after confirmation.
	en, err := h.svc.ResetTOTP(ctx, "ops")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.svc.Authenticate(ctx, cookie); !errors.Is(err, operator.ErrUnauthorized) {
		t.Fatalf("a session after the reset: %v", err)
	}
	h.clock.Advance(time.Minute)
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: fresh[0]}); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("a backup code after the reset: %v", err)
	}
	if _, err := h.svc.ConfirmTOTP(ctx, "ops", operator.CodeAt(en.Secret, h.clock.Now())); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: operator.CodeAt(secret, h.clock.Now())}); !errors.Is(err, operator.ErrBadCredentials) {
		t.Fatalf("the old secret after the reset: %v", err)
	}
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: operator.CodeAt(en.Secret, h.clock.Now())}); err != nil {
		t.Fatalf("the new secret: %v", err)
	}
}

func TestOperatorSecretsAreEncryptedAndRotate(t *testing.T) {
	h := newOpHarness(t)
	ctx := context.Background()
	secret, _ := h.enroll(t, "ops")
	o, err := h.store.OperatorByHandle(ctx, "ops")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(o.TOTP.Ciphertext), secret) || o.TOTP.KeyID != "k1" {
		t.Fatalf("stored TOTP: %+v", o.TOTP)
	}
	// Another row cannot use this ciphertext.
	if _, err := h.keys.Open(o.TOTP, secretbox.AAD("operator", ids.Nil.String(), "", "totp")); err == nil {
		t.Fatal("the secret opened for another row")
	}

	k2, _ := secretbox.NewKey("k2")
	// The keyring with a new active key and the old one.
	ring, err := secretbox.Parse(k2 + "," + h.key)
	if err != nil {
		t.Fatal(err)
	}
	h.svc.Keys = ring
	if n, err := h.svc.RotateSecrets(ctx); err != nil || n != 1 {
		t.Fatalf("rotate: %d %v", n, err)
	}
	if n, _ := h.svc.RotateSecrets(ctx); n != 0 {
		t.Fatalf("a second rotation changed %d", n)
	}
	// Only the new key now.
	only, _ := secretbox.Parse(k2)
	h.svc.Keys = only
	h.clock.Advance(time.Minute)
	if _, _, _, err := h.svc.Login(ctx, operator.LoginInput{Handle: "ops", Password: opPassword, Code: operator.CodeAt(secret, h.clock.Now())}); err != nil {
		t.Fatalf("login after rotation with the new key only: %v", err)
	}
}
