// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"colonyhall/internal/modules/operator"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/db/opscope"
	"colonyhall/internal/platform/secretbox"
)

// testOperator: the platform operator's store (round 51). Everything runs
// in the operator scope.
func testOperator(t *testing.T, b Backend) {
	ctx := opscope.Context(context.Background())
	st := b.Operator
	sealed := secretbox.Sealed{KeyID: "k1", Nonce: []byte("123456789012"), Ciphertext: []byte("cipher")}
	o := operator.Operator{ID: gen.New(), Handle: "ops", DisplayName: "Ops", PasswordHash: "$argon2id$x", TOTP: sealed,
		State: operator.StatePending, CreatedAt: t0}
	must(t, st.CreateOperator(ctx, o))
	dup := o
	dup.ID = gen.New()
	if err := st.CreateOperator(ctx, dup); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("a duplicate handle: %v", err)
	}
	got, err := st.OperatorByHandle(ctx, "ops")
	if err != nil || got.ID != o.ID || got.TOTP.KeyID != "k1" || string(got.TOTP.Ciphertext) != "cipher" || got.State != operator.StatePending {
		t.Fatalf("by handle: %+v %v", got, err)
	}
	if _, err := st.OperatorByHandle(ctx, "nobody"); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}

	// TOTP steps only move forward.
	if ok, err := st.AdvanceTOTPStep(ctx, o.ID, 100); err != nil || !ok {
		t.Fatalf("advance: %v %v", ok, err)
	}
	for _, step := range []int64{100, 99} {
		if ok, err := st.AdvanceTOTPStep(ctx, o.ID, step); err != nil || ok {
			t.Fatalf("step %d again: %v %v", step, ok, err)
		}
	}
	at := t0.Add(time.Minute)
	must(t, st.SetState(ctx, o.ID, operator.StateActive, &at))
	must(t, st.SetLastLogin(ctx, o.ID, at))
	if got, _ := st.OperatorByID(ctx, o.ID); got.State != operator.StateActive || got.TOTPLastStep != 100 || got.ActivatedAt == nil || got.LastLoginAt == nil {
		t.Fatalf("after activation: %+v", got)
	}
	// A new secret resets the step; resealing keeps it.
	must(t, st.SetTOTP(ctx, o.ID, secretbox.Sealed{KeyID: "k1", Nonce: []byte("abcdefghijkl"), Ciphertext: []byte("new")}, operator.StatePending))
	if got, _ := st.OperatorByID(ctx, o.ID); got.TOTPLastStep != 0 || got.State != operator.StatePending || string(got.TOTP.Ciphertext) != "new" {
		t.Fatalf("after a new secret: %+v", got)
	}
	if ok, err := st.AdvanceTOTPStep(ctx, o.ID, 5); err != nil || !ok {
		t.Fatalf("advance after a new secret: %v %v", ok, err)
	}
	must(t, st.ResealTOTP(ctx, o.ID, secretbox.Sealed{KeyID: "k2", Nonce: []byte("mnopqrstuvwx"), Ciphertext: []byte("resealed")}))
	if got, _ := st.OperatorByID(ctx, o.ID); got.TOTP.KeyID != "k2" || got.TOTPLastStep != 5 {
		t.Fatalf("after resealing: %+v", got)
	}
	if list, err := st.ListOperators(ctx); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}

	// Backup codes: once each; replacing voids the old ones.
	codes := []operator.BackupCode{{ID: gen.New(), Hash: []byte("h1")}, {ID: gen.New(), Hash: []byte("h2")}}
	must(t, st.ReplaceBackupCodes(ctx, o.ID, codes, t0))
	if n, err := st.UnusedBackupCodes(ctx, o.ID); err != nil || n != 2 {
		t.Fatalf("unused: %d %v", n, err)
	}
	if ok, err := st.UseBackupCode(ctx, o.ID, []byte("h1"), at); err != nil || !ok {
		t.Fatalf("use: %v %v", ok, err)
	}
	if ok, _ := st.UseBackupCode(ctx, o.ID, []byte("h1"), at); ok {
		t.Fatal("a backup code was used twice")
	}
	must(t, st.ReplaceBackupCodes(ctx, o.ID, []operator.BackupCode{{ID: gen.New(), Hash: []byte("h3")}}, at))
	if ok, _ := st.UseBackupCode(ctx, o.ID, []byte("h2"), at); ok {
		t.Fatal("a replaced backup code was used")
	}
	if n, _ := st.UnusedBackupCodes(ctx, o.ID); n != 1 {
		t.Fatalf("after replacing: %d", n)
	}

	// Sessions.
	s1 := operator.Session{ID: gen.New(), OperatorID: o.ID, TokenHash: []byte("t1"), CSRFToken: "c", CreatedAt: t0, LastSeenAt: t0,
		ExpiresAt: t0.Add(operator.SessionTTL), RemoteAddr: "192.0.2.1", UserAgent: "ua"}
	s2 := s1
	s2.ID, s2.TokenHash = gen.New(), []byte("t2")
	must(t, st.CreateSession(ctx, s1))
	must(t, st.CreateSession(ctx, s2))
	must(t, st.TouchSession(ctx, s1.ID, at))
	if got, err := st.SessionByTokenHash(ctx, []byte("t1")); err != nil || !got.LastSeenAt.Equal(at) || got.RemoteAddr != "192.0.2.1" || got.RevokedAt != nil {
		t.Fatalf("session: %+v %v", got, err)
	}
	must(t, st.RevokeSession(ctx, s1.ID, at))
	if got, _ := st.SessionByTokenHash(ctx, []byte("t1")); got.RevokedAt == nil {
		t.Fatal("revoke")
	}
	must(t, st.RevokeSessions(ctx, o.ID, at))
	if got, _ := st.SessionByTokenHash(ctx, []byte("t2")); got.RevokedAt == nil {
		t.Fatal("revoke all")
	}
	if n, err := st.DeleteEndedSessions(ctx, at.Add(time.Hour)); err != nil || n != 2 {
		t.Fatalf("cleanup: %d %v", n, err)
	}
	if _, err := st.SessionByTokenHash(ctx, []byte("t1")); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("after cleanup: %v", err)
	}

	// Audit, newest first, paged by time.
	for i := range 3 {
		must(t, st.AppendAudit(ctx, operator.AuditEntry{ID: gen.New(), OperatorID: o.ID, Action: "test", WorkspaceID: gen.New(),
			Payload: map[string]any{"n": float64(i)}, RemoteAddr: "192.0.2.1", CreatedAt: t0.Add(time.Duration(i) * time.Minute)}))
	}
	must(t, st.AppendAudit(ctx, operator.AuditEntry{ID: gen.New(), Action: "cli", Payload: map[string]any{}, CreatedAt: t0.Add(time.Hour)}))
	all, err := st.ListAudit(ctx, time.Time{}, 10)
	if err != nil || len(all) != 4 || all[0].Action != "cli" || all[0].OperatorID != [16]byte{} || all[1].Payload["n"] != float64(2) {
		t.Fatalf("audit: %+v %v", all, err)
	}
	if page, _ := st.ListAudit(ctx, t0.Add(2*time.Minute), 10); len(page) != 2 || page[0].Payload["n"] != float64(1) {
		t.Fatalf("audit page: %+v", page)
	}

	// Round 51b: plans; names are unique; the current ones first, by storage.
	p1 := operator.Plan{ID: gen.New(), Name: "Nagy", StorageBytes: 50 << 30, CPUCores: 8, MemoryBytes: 8 << 30, CreatedAt: t0, UpdatedAt: t0}
	p2 := operator.Plan{ID: gen.New(), Name: "Kicsi", StorageBytes: 15 << 30, CPUCores: 2, MemoryBytes: 2 << 30, CreatedAt: t0, UpdatedAt: t0}
	must(t, st.CreatePlan(ctx, p1))
	must(t, st.CreatePlan(ctx, p2))
	if err := st.CreatePlan(ctx, operator.Plan{ID: gen.New(), Name: "Nagy", StorageBytes: 1, CPUCores: 1, MemoryBytes: 1, CreatedAt: t0, UpdatedAt: t0}); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("a taken plan name: %v", err)
	}
	if got, err := st.PlanByID(ctx, p1.ID); err != nil || got.Name != "Nagy" || got.CPUCores != 8 || got.MemoryBytes != 8<<30 || got.ArchivedAt != nil {
		t.Fatalf("plan by id: %+v %v", got, err)
	}
	if _, err := st.PlanByID(ctx, gen.New()); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("unknown plan: %v", err)
	}
	if list, err := st.ListPlans(ctx); err != nil || len(list) != 2 || list[0].ID != p2.ID || list[1].ID != p1.ID {
		t.Fatalf("plans by storage: %+v %v", list, err)
	}
	archived := t0.Add(time.Hour)
	p2.ArchivedAt, p2.StorageBytes, p2.UpdatedAt = &archived, 20<<30, archived
	must(t, st.UpdatePlan(ctx, p2))
	if list, _ := st.ListPlans(ctx); len(list) != 2 || list[0].ID != p1.ID || list[1].ArchivedAt == nil || list[1].StorageBytes != 20<<30 {
		t.Fatalf("an archived plan goes last: %+v", list)
	}
	p2.Name = "Nagy"
	if err := st.UpdatePlan(ctx, p2); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("renaming to a taken name: %v", err)
	}
}
