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

// testInvitesAndSuspension: the owner's one-time invites and the
// workspace's suspension (round 51).
func testInvitesAndSuspension(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Identity

	if _, err := st.LatestInvite(ctx, f.ws.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("no invite yet: %v", err)
	}
	first := identity.Invite{ID: gen.New(), WorkspaceID: f.ws.ID, TokenHash: identity.HashSecret("first"), Role: identity.RoleOwner,
		CreatedByOperator: gen.New(), CreatedAt: t0, ExpiresAt: t0.Add(identity.InviteTTL)}
	must(t, st.CreateInvite(ctx, first))
	got, err := st.InviteByTokenHash(ctx, identity.HashSecret("first"))
	if err != nil || got.ID != first.ID || got.CreatedByOperator != first.CreatedByOperator || got.Role != identity.RoleOwner || !got.Open(t0) {
		t.Fatalf("by hash: %+v %v", got, err)
	}
	if _, err := st.InviteByTokenHash(ctx, identity.HashSecret("nope")); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("unknown hash: %v", err)
	}

	// A new invite revokes the open one.
	later := t0.Add(time.Hour)
	must(t, st.RevokeOpenInvites(ctx, f.ws.ID, later))
	second := first
	second.ID, second.TokenHash, second.CreatedAt, second.ExpiresAt = gen.New(), identity.HashSecret("second"), later, later.Add(identity.InviteTTL)
	must(t, st.CreateInvite(ctx, second))
	if got, _ := st.InviteByTokenHash(ctx, identity.HashSecret("first")); got.RevokedAt == nil || got.Open(later) {
		t.Fatalf("the first invite must be revoked: %+v", got)
	}
	if ok, err := st.UseInvite(ctx, first.ID, later); err != nil || ok {
		t.Fatalf("a revoked invite was used: %v %v", ok, err)
	}
	if latest, err := st.LatestInvite(ctx, f.ws.ID); err != nil || latest.ID != second.ID {
		t.Fatalf("latest: %+v %v", latest, err)
	}

	// Expired: not usable.
	if ok, err := st.UseInvite(ctx, second.ID, second.ExpiresAt); err != nil || ok {
		t.Fatalf("an expired invite was used: %v %v", ok, err)
	}
	// Once only.
	if ok, err := st.UseInvite(ctx, second.ID, later); err != nil || !ok {
		t.Fatalf("use: %v %v", ok, err)
	}
	if ok, err := st.UseInvite(ctx, second.ID, later); err != nil || ok {
		t.Fatalf("an invite was used twice: %v %v", ok, err)
	}
	must(t, st.SetInviteUsedBy(ctx, second.ID, f.human.ID))
	if got, _ := st.InviteByTokenHash(ctx, identity.HashSecret("second")); got.UsedAt == nil || got.UsedBy != f.human.ID {
		t.Fatalf("used: %+v", got)
	}

	// Suspension keeps its reason and time; restoring clears them.
	must(t, st.SetWorkspaceState(ctx, f.ws.ID, identity.WorkspaceSuspended, "unpaid", &later))
	ws, err := st.WorkspaceByID(ctx, f.ws.ID)
	if err != nil || ws.State != identity.WorkspaceSuspended || ws.SuspendedReason != "unpaid" || ws.SuspendedAt == nil || !ws.SuspendedAt.Equal(later) {
		t.Fatalf("suspended: %+v %v", ws, err)
	}
	if w, err := st.WorkspaceBySlug(context.Background(), ws.Slug); err != nil || w.State != identity.WorkspaceSuspended {
		t.Fatalf("the slug lookup sees the state: %+v %v", w, err)
	}
	must(t, st.SetWorkspaceState(ctx, f.ws.ID, identity.WorkspaceActive, "", nil))
	if ws, _ := st.WorkspaceByID(ctx, f.ws.ID); ws.State != identity.WorkspaceActive || ws.SuspendedReason != "" || ws.SuspendedAt != nil {
		t.Fatalf("restored: %+v", ws)
	}
}
