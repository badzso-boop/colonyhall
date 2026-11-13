package storetest

import (
	"context"
	"testing"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

// testAuthFailures: the login and release limits (round 40). The counts
// work before anyone is authenticated (no scope, or another workspace's);
// the lockout list is the workspace's own.
func testAuthFailures(t *testing.T, b Backend) {
	f := setup(t, b)
	other := identity.Workspace{ID: gen.New(), Name: "other", CreatedAt: t0, Slug: "other"}
	must(t, b.Identity.CreateWorkspace(db.WithWorkspace(context.Background(), other.ID), other))
	unscoped := context.Background()
	inWS, inOther := db.WithWorkspace(unscoped, f.ws.ID), db.WithWorkspace(unscoped, other.ID)

	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	until := at(20)
	rec := func(ctx context.Context, ws ids.ID, scope, handle, ip string, m int, locked *time.Time) {
		t.Helper()
		must(t, b.Identity.RecordAuthFailure(ctx, identity.AuthFailure{ID: gen.New(), WorkspaceID: ws, Scope: scope,
			Handle: handle, IP: ip, At: at(m), LockedUntil: locked}))
	}
	rec(unscoped, f.ws.ID, identity.LimitLogin, "norbi", "192.0.2.1", 1, nil)
	rec(inOther, ids.Nil, identity.LimitLogin, "ghost", "192.0.2.1", 2, nil) // an unknown handle, from another scope
	rec(unscoped, f.ws.ID, identity.LimitLogin, "norbi", "192.0.2.2", 3, &until)
	rec(unscoped, f.ws.ID, identity.LimitEmergencyClear, "norbi", "192.0.2.1", 4, nil) // another scope's counter
	rec(unscoped, ids.Nil, identity.LimitLogin, "ghost", "198.51.100.1", 5, &until)
	rec(unscoped, f.ws.ID, identity.LimitLogin, "norbi", "192.0.2.1", -60, nil) // before the window

	// A fresh read from any scope (a restarted server reads the same rows).
	for _, ctx := range []context.Context{unscoped, inWS, inOther} {
		got, err := b.Identity.AuthFailures(ctx, identity.LimitLogin, "norbi", "192.0.2.1", t0)
		must(t, err)
		if len(got.ByHandle) != 2 || !got.ByHandle[0].Equal(at(1)) || !got.ByHandle[1].Equal(at(3)) {
			t.Fatalf("by handle: %v", got.ByHandle)
		}
		if len(got.ByIP) != 2 || !got.ByIP[0].Equal(at(1)) || !got.ByIP[1].Equal(at(2)) {
			t.Fatalf("by ip: %v", got.ByIP)
		}
	}
	if got, err := b.Identity.AuthFailures(unscoped, identity.LimitLogin, "", "", t0); err != nil || len(got.ByHandle)+len(got.ByIP) != 0 {
		t.Fatalf("empty keys match nothing: %+v %v", got, err)
	}

	// A success clears the handle, not the address.
	must(t, b.Identity.ClearAuthFailures(inOther, identity.LimitLogin, "norbi", at(6)))
	got, err := b.Identity.AuthFailures(unscoped, identity.LimitLogin, "norbi", "192.0.2.1", t0)
	must(t, err)
	if len(got.ByHandle) != 0 || len(got.ByIP) != 2 {
		t.Fatalf("after clearing: %+v", got)
	}
	got, err = b.Identity.AuthFailures(unscoped, identity.LimitEmergencyClear, "norbi", "", t0)
	if err != nil || len(got.ByHandle) != 1 {
		t.Fatalf("another scope was cleared too: %+v %v", got, err)
	}

	// Lockouts: the workspace sees its own, never another's; the
	// unattributed ones only through their own call.
	lo, err := b.Identity.WorkspaceLockouts(inWS, f.ws.ID, t0)
	must(t, err)
	if len(lo) != 1 || lo[0].Handle != "norbi" || lo[0].IP != "192.0.2.2" || !lo[0].Until.Equal(until) || !lo[0].At.Equal(at(3)) {
		t.Fatalf("workspace lockouts: %+v", lo)
	}
	if lo, err := b.Identity.WorkspaceLockouts(inOther, f.ws.ID, t0); err != nil || len(lo) != 0 {
		t.Fatalf("another workspace's scope read the lockouts: %+v %v", lo, err)
	}
	if lo, err := b.Identity.WorkspaceLockouts(inOther, other.ID, t0); err != nil || len(lo) != 0 {
		t.Fatalf("the other workspace has none: %+v %v", lo, err)
	}
	lo, err = b.Identity.UnattributedLockouts(inOther, t0)
	must(t, err)
	if len(lo) != 1 || lo[0].Handle != "ghost" {
		t.Fatalf("unattributed lockouts: %+v", lo)
	}

	// The cleanup deletes the old rows (the maintenance loop runs it in
	// system mode; a workspace scope reaches its own rows).
	n, err := b.Identity.DeleteAuthFailures(inWS, t0)
	must(t, err)
	if n != 1 {
		t.Fatalf("deleted %d, want 1", n)
	}
	got, err = b.Identity.AuthFailures(unscoped, identity.LimitLogin, "norbi", "192.0.2.1", t0.Add(-2*time.Hour))
	must(t, err)
	if len(got.ByIP) != 2 {
		t.Fatalf("the cleanup deleted recent rows: %+v", got)
	}
}
