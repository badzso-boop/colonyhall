// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/db"
)

// testBreakGlass: round 51b. The operator's break-glass accesses: the
// handoff is used once, an access closes once, the list is newest first,
// the overdue ones are found across workspaces (system mode), the operator
// actor is remembered, and a session carries its access.
func testBreakGlass(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	op := identity.Actor{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: identity.KindHuman, Handle: "colonyhall-operator",
		DisplayName: "Operator", Role: identity.RoleAdmin, State: "active", CreatedAt: t0}
	must(t, b.Identity.CreateActor(ctx, op))

	if _, err := b.Identity.BreakGlassActor(ctx, f.ws.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("no access yet: %v", err)
	}
	mk := func(opened time.Time, hash string) identity.BreakGlass {
		return identity.BreakGlass{ID: gen.New(), WorkspaceID: f.ws.ID, OperatorID: gen.New(), OperatorHandle: "ops", ActorID: op.ID,
			Reason: "a reason long enough", OpenedAt: opened, ExpiresAt: opened.Add(time.Hour), HandoffHash: []byte(hash),
			HandoffExpiresAt: opened.Add(time.Minute)}
	}
	b1, b2 := mk(t0, "h1"), mk(t0.Add(time.Minute), "h2")
	must(t, b.Identity.CreateBreakGlass(ctx, b1))
	must(t, b.Identity.CreateBreakGlass(ctx, b2))

	if got, err := b.Identity.BreakGlassByID(ctx, b1.ID); err != nil || got.OperatorHandle != "ops" || got.ActorID != op.ID || !got.Open(t0) {
		t.Fatalf("by id: %+v %v", got, err)
	}
	if got, err := b.Identity.BreakGlassByHandoffHash(ctx, []byte("h2")); err != nil || got.ID != b2.ID {
		t.Fatalf("by handoff: %+v %v", got, err)
	}
	if _, err := b.Identity.BreakGlassByHandoffHash(ctx, []byte("nope")); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("unknown handoff: %v", err)
	}
	// The handoff: once, and not after its minute.
	if ok, err := b.Identity.UseBreakGlassHandoff(ctx, b1.ID, t0.Add(2*time.Minute)); err != nil || ok {
		t.Fatalf("an expired handoff: %v %v", ok, err)
	}
	if ok, err := b.Identity.UseBreakGlassHandoff(ctx, b1.ID, t0.Add(30*time.Second)); err != nil || !ok {
		t.Fatalf("handoff: %v %v", ok, err)
	}
	if ok, _ := b.Identity.UseBreakGlassHandoff(ctx, b1.ID, t0.Add(40*time.Second)); ok {
		t.Fatal("the handoff worked twice")
	}
	// Newest first; the actor is remembered.
	if list, err := b.Identity.ListBreakGlass(ctx, f.ws.ID, 10); err != nil || len(list) != 2 || list[0].ID != b2.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	if id, err := b.Identity.BreakGlassActor(ctx, f.ws.ID); err != nil || id != op.ID {
		t.Fatalf("the operator actor: %v %v", id, err)
	}
	if yes, err := b.Identity.IsBreakGlassActor(ctx, op.ID); err != nil || !yes {
		t.Fatalf("is the operator actor: %v %v", yes, err)
	}
	if yes, _ := b.Identity.IsBreakGlassActor(ctx, f.human.ID); yes {
		t.Fatal("the owner taken for the operator actor")
	}
	// A session carries its access.
	sess := identity.Session{ID: gen.New(), ActorID: op.ID, TokenHash: []byte("bg-session"), CSRFToken: "c", CreatedAt: t0,
		ExpiresAt: b1.ExpiresAt, BreakGlassID: b1.ID}
	must(t, b.Identity.CreateSession(ctx, sess))
	if got, a, err := b.Identity.SessionByTokenHash(ctx, []byte("bg-session")); err != nil || got.BreakGlassID != b1.ID || a.ID != op.ID {
		t.Fatalf("the break-glass session: %+v %v", got, err)
	}
	// Closed once; the overdue list, across workspaces, skips it.
	if ok, err := b.Identity.CloseBreakGlass(ctx, b1.ID, identity.BreakGlassClosedByOwner, t0.Add(time.Minute)); err != nil || !ok {
		t.Fatalf("close: %v %v", ok, err)
	}
	if ok, _ := b.Identity.CloseBreakGlass(ctx, b1.ID, identity.BreakGlassClosedByOperator, t0.Add(2*time.Minute)); ok {
		t.Fatal("closed twice")
	}
	if got, _ := b.Identity.BreakGlassByID(ctx, b1.ID); got.ClosedBy != identity.BreakGlassClosedByOwner || got.ClosedAt == nil || got.Open(t0) {
		t.Fatalf("closed: %+v", got)
	}
	over, err := b.Identity.OverdueBreakGlass(b.System(context.Background()), t0.Add(3*time.Hour))
	if err != nil || len(over) != 1 || over[0].ID != b2.ID {
		t.Fatalf("overdue: %+v %v", over, err)
	}
	if over, _ := b.Identity.OverdueBreakGlass(b.System(context.Background()), t0.Add(time.Minute)); len(over) != 0 {
		t.Fatalf("overdue before its time: %+v", over)
	}
}
