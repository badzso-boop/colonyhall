// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

// Service-level tests of the login limits (round 40): a restarted server is
// a fresh identity.Service on the same store.

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	evmem "colonyhall/internal/eventlog/memory"
	"colonyhall/internal/modules/identity"
	idmem "colonyhall/internal/modules/identity/memory"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clientip"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

const goodPassword = "correct horse battery staple"

type limitEnv struct {
	store  *idmem.Store
	events *evmem.Store
	clk    *clock.Manual
	gen    *ids.Sequential
	owner  identity.Actor
}

func newLimitEnv(t *testing.T) *limitEnv {
	t.Helper()
	e := &limitEnv{store: idmem.New(), events: evmem.New(), clk: clock.NewManual(time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)), gen: &ids.Sequential{}}
	ws, owner, _, err := e.service(nil).Bootstrap(context.Background(), "acme", "acme", "alice", "Alice", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := db.WithWorkspace(context.Background(), ws.ID)
	if err := e.service(nil).ResetPassword(ctx, owner.ID, goodPassword); err != nil {
		t.Fatal(err)
	}
	e.owner = owner
	return e
}

// service is a fresh Service on the same store: a restarted server.
func (e *limitEnv) service(allow []netip.Prefix) *identity.Service {
	return &identity.Service{Store: e.store, Events: e.events, Bus: bus.NewInProc(), Tx: &db.Memory{}, IDs: e.gen, Clock: e.clk,
		LoginAllowlist: allow}
}

func fromIP(ip string) context.Context {
	return clientip.With(context.Background(), netip.MustParseAddr(ip))
}

func tryLogin(svc *identity.Service, ip, handle, password string) error {
	_, _, _, err := svc.Login(fromIP(ip), identity.LoginInput{Handle: handle, Password: password})
	return err
}

func TestLockoutSurvivesRestart(t *testing.T) {
	e := newLimitEnv(t)
	svc := e.service(nil)
	for i := range 5 {
		if err := tryLogin(svc, "192.0.2.1", "alice", "wrong password!!"); !errors.Is(err, identity.ErrBadCredentials) {
			t.Fatalf("failure %d: %v", i+1, err)
		}
	}
	e.clk.Advance(time.Minute)
	// A new process: the old in-memory limiter forgot everything here.
	restarted := e.service(nil)
	err := tryLogin(restarted, "192.0.2.1", "alice", goodPassword)
	var locked *identity.LockedOutError
	if !errors.As(err, &locked) || !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("after a restart the lockout is gone: %v", err)
	}
	// The first failure was at 09:00, the window is 15 minutes: 14 minutes left.
	if locked.RetryAfter() != 14*time.Minute {
		t.Fatalf("retry after %v", locked.RetryAfter())
	}
	e.clk.Advance(14 * time.Minute)
	if err := tryLogin(restarted, "192.0.2.1", "alice", goodPassword); err != nil {
		t.Fatalf("after the window the right password works: %v", err)
	}
}

func TestHandleWindowSlides(t *testing.T) {
	e := newLimitEnv(t)
	svc := e.service(nil)
	// Four failures, then a fifth ten minutes later: still under the limit
	// until the fifth; the window then slides from the first.
	for range 4 {
		_ = tryLogin(svc, "192.0.2.1", "alice", "wrong password!!")
	}
	e.clk.Advance(10 * time.Minute)
	_ = tryLogin(svc, "192.0.2.2", "alice", "wrong password!!") // another address: the handle counts
	if err := tryLogin(svc, "192.0.2.3", "alice", goodPassword); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("five failures of the handle from three addresses: %v", err)
	}
	e.clk.Advance(5*time.Minute + time.Second) // the first four left the window
	if err := tryLogin(svc, "192.0.2.3", "alice", goodPassword); err != nil {
		t.Fatalf("the window slid: %v", err)
	}
	// The case of a handle does not open a new counter.
	e.clk.Advance(time.Hour)
	for _, h := range []string{"ALICE", "Alice", " alice", "@alice", "aLiCe"} {
		_ = tryLogin(svc, "192.0.2.9", h, "wrong password!!")
	}
	if err := tryLogin(svc, "192.0.2.10", "alice", goodPassword); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("spellings of one handle share its counter: %v", err)
	}
	e.clk.Advance(time.Hour)
	// A success clears the handle's count.
	for range 4 {
		_ = tryLogin(svc, "192.0.2.1", "alice", "wrong password!!")
	}
	if err := tryLogin(svc, "192.0.2.1", "alice", goodPassword); err != nil {
		t.Fatalf("the earlier failures were cleared by the success: %v", err)
	}
}

func TestIPWindowCountsEveryHandle(t *testing.T) {
	e := newLimitEnv(t)
	svc := e.service(nil)
	// Password spraying: one address, many handles (most do not exist).
	for i := range 20 {
		h := []string{"bob", "carol", "dave", "eve"}[i%4]
		_ = tryLogin(svc, "198.51.100.7", h, "wrong password!!")
	}
	if err := tryLogin(svc, "198.51.100.7", "alice", goodPassword); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("the address reached its limit, alice is refused from it: %v", err)
	}
	// alice herself is not locked: from another address she logs in.
	if err := tryLogin(svc, "192.0.2.50", "alice", goodPassword); err != nil {
		t.Fatalf("the handle was never locked: %v", err)
	}
	// A success from the sprayer's address does not reset the address's count.
	if err := tryLogin(svc, "198.51.100.7", "alice", goodPassword); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("still locked: %v", err)
	}
}

func TestAllowlistIsNeverLockedOut(t *testing.T) {
	e := newLimitEnv(t)
	allow, err := clientip.ParseList("127.0.0.1/32, 100.64.0.0/10")
	if err != nil {
		t.Fatal(err)
	}
	svc := e.service(allow)
	for range 8 {
		_ = tryLogin(svc, "198.51.100.9", "alice", "wrong password!!")
	}
	if err := tryLogin(svc, "198.51.100.9", "alice", goodPassword); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("an address outside the list is locked: %v", err)
	}
	if err := tryLogin(svc, "100.64.0.7", "alice", "wrong password!!"); !errors.Is(err, identity.ErrBadCredentials) {
		t.Fatalf("an allowlisted address still has its password checked: %v", err)
	}
	if err := tryLogin(svc, "100.64.0.7", "alice", goodPassword); err != nil {
		t.Fatalf("an allowlisted address is never locked out: %v", err)
	}
	// Its failures are still counted (and audited).
	times, err := e.store.AuthFailures(context.Background(), identity.LimitLogin, "", "100.64.0.7", e.clk.Now().Add(-time.Hour))
	if err != nil || len(times.ByIP) != 1 {
		t.Fatalf("the allowlisted failure was not recorded: %+v %v", times, err)
	}
}

func TestLockoutIsAuditedWithoutPassword(t *testing.T) {
	e := newLimitEnv(t)
	svc := e.service(nil)
	for range 5 {
		_ = tryLogin(svc, "192.0.2.1", "alice", "wrong password!!")
	}
	evs, err := e.events.Since(db.WithWorkspace(context.Background(), e.owner.WorkspaceID), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, ev := range evs {
		if ev.Type != identity.EventLockedOut {
			continue
		}
		found++
		p := string(ev.Payload)
		for _, want := range []string{`"handle":"alice"`, `"ip":"192.0.2.1"`, `"scope":"login"`, `"until":"2026-11-13T09:15:00Z"`} {
			if !hasSub(p, want) {
				t.Errorf("payload %s lacks %s", p, want)
			}
		}
		if hasSub(p, "wrong password") {
			t.Fatalf("the password reached the audit log: %s", p)
		}
	}
	if found != 1 {
		t.Fatalf("%d lockout events, want 1", found)
	}
	lo, err := svc.Lockouts(db.WithWorkspace(context.Background(), e.owner.WorkspaceID), e.owner, e.clk.Now().Add(-24*time.Hour), false)
	if err != nil || len(lo) != 1 || lo[0].Handle != "alice" {
		t.Fatalf("lockouts %+v %v", lo, err)
	}
}

func TestReleaseAndPasswordChangeHaveTheirOwnLimits(t *testing.T) {
	e := newLimitEnv(t)
	svc := e.service(nil)
	ctx := db.WithWorkspace(fromIP("192.0.2.1"), e.owner.WorkspaceID)
	for range 5 {
		_ = tryLogin(svc, "192.0.2.1", "alice", "wrong password!!")
	}
	// The login is locked; the release is not.
	if err := svc.EmergencyStop(ctx, e.owner); err != nil {
		t.Fatal(err)
	}
	if err := svc.ClearEmergency(ctx, e.owner, goodPassword); err != nil {
		t.Fatalf("a login lockout must not block the release: %v", err)
	}
	// Five wrong current passwords lock the password change.
	for i := range 5 {
		if err := svc.SetPassword(ctx, e.owner, ids.Nil, e.owner.ID, "wrong password!!", "a brand new password"); !errors.Is(err, identity.ErrBadCredentials) {
			t.Fatalf("try %d: %v", i+1, err)
		}
	}
	if err := svc.SetPassword(ctx, e.owner, ids.Nil, e.owner.ID, goodPassword, "a brand new password"); !errors.Is(err, identity.ErrTooManyAttempts) {
		t.Fatalf("the sixth guess of the current password must wait: %v", err)
	}
}

func hasSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
