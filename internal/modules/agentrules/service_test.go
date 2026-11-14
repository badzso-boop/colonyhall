// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package agentrules

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	evmem "colonyhall/internal/eventlog/memory"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/platform/bus"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
)

type fakeStore struct {
	rules map[int]Rules
}

func newFakeStore() *fakeStore {
	return &fakeStore{rules: make(map[int]Rules)}
}

func (s *fakeStore) Insert(_ context.Context, r Rules) error {
	s.rules[r.Version] = r
	return nil
}

func (s *fakeStore) GetLatest(_ context.Context, _ ids.ID) (Rules, error) {
	if len(s.rules) == 0 {
		return Rules{}, db.ErrNotFound
	}
	var highest int
	for v := range s.rules {
		if v > highest {
			highest = v
		}
	}
	return s.rules[highest], nil
}

func (s *fakeStore) GetVersion(_ context.Context, _ ids.ID, version int) (Rules, error) {
	r, ok := s.rules[version]
	if !ok {
		return Rules{}, db.ErrNotFound
	}
	return r, nil
}

func (s *fakeStore) ListHistory(_ context.Context, _ ids.ID) ([]RulesMeta, error) {
	var meta []RulesMeta
	for _, r := range s.rules {
		meta = append(meta, RulesMeta{
			Version:   r.Version,
			CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt,
		})
	}
	sort.Slice(meta, func(i, j int) bool {
		return meta[i].Version > meta[j].Version
	})
	return meta, nil
}

type fakeDirectory struct {
	admins    []identity.Actor
	workspace identity.Workspace
}

func (f *fakeDirectory) Admins(_ context.Context, _ ids.ID) ([]identity.Actor, error) {
	return f.admins, nil
}

func (f *fakeDirectory) WorkspaceByID(_ context.Context, _ ids.ID) (identity.Workspace, error) {
	return f.workspace, nil
}

func TestRulesService(t *testing.T) {
	ctx := context.Background()
	clk := clock.NewManual(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	gen := &ids.Sequential{}
	wsID := gen.New()
	adminID := gen.New()
	memberID := gen.New()
	agentID := gen.New()

	dir := &fakeDirectory{
		admins: []identity.Actor{
			{ID: adminID, WorkspaceID: wsID, Kind: identity.KindHuman, Handle: "bob", Role: identity.RoleAdmin},
			{ID: gen.New(), WorkspaceID: wsID, Kind: identity.KindHuman, Handle: "alice", Role: identity.RoleOwner},
		},
		workspace: identity.Workspace{ID: wsID, Name: "Acme Corp"},
	}

	st := newFakeStore()
	events := evmem.New()
	b := bus.NewInProc()

	svc := &Service{
		Store:     st,
		Directory: dir,
		Events:    events,
		Bus:       b,
		Tx:        &db.Memory{},
		IDs:       gen,
		Clock:     clk,
	}

	// 1. Version 0 when empty
	latest, err := svc.GetLatest(ctx, wsID)
	if err != nil {
		t.Fatalf("GetLatest on empty store: %v", err)
	}
	if latest.Version != 0 || latest.Text != DefaultRulesText {
		t.Fatalf("expected version 0 and default text, got version %d", latest.Version)
	}

	v0, err := svc.GetVersion(ctx, wsID, 0)
	if err != nil {
		t.Fatalf("GetVersion(0): %v", err)
	}
	if v0.Version != 0 || v0.Text != DefaultRulesText {
		t.Fatalf("expected version 0, got %d", v0.Version)
	}

	if _, err := svc.GetVersion(ctx, wsID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing version 1, got: %v", err)
	}

	// 2. PUT by non-admin human -> 403
	nonAdmin := identity.Actor{ID: memberID, WorkspaceID: wsID, Kind: identity.KindHuman, Role: identity.RoleMember}
	if _, err := svc.Update(ctx, nonAdmin, "new rules"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin human, got: %v", err)
	}

	// 3. PUT by agent -> 403
	agentActor := identity.Actor{ID: agentID, WorkspaceID: wsID, Kind: identity.KindAgent, Handle: "my-agent", Role: identity.RoleMember}
	if _, err := svc.Update(ctx, agentActor, "new rules"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for agent actor, got: %v", err)
	}

	// 4. PUT by admin human -> new version 1
	adminActor := identity.Actor{ID: adminID, WorkspaceID: wsID, Kind: identity.KindHuman, Handle: "bob", Role: identity.RoleAdmin}
	customText := "You are @{handle} in {workspace}. Admins: {admins}. Unknown: {unknown_placeholder}."
	u1, err := svc.Update(ctx, adminActor, customText)
	if err != nil {
		t.Fatalf("Update by admin: %v", err)
	}
	if u1.Version != 1 || u1.Text != customText {
		t.Fatalf("unexpected u1: %+v", u1)
	}

	// Verify event logged
	evs, err := events.Since(ctx, 0, 10)
	if err != nil || len(evs) != 1 {
		t.Fatalf("expected 1 event, got %d (err: %v)", len(evs), err)
	}
	if evs[0].Type != EventRulesUpdated {
		t.Fatalf("unexpected event type: %s", evs[0].Type)
	}

	// 5. Placeholders rendered per agent
	v, rendered, err := svc.RenderInstructions(ctx, agentActor)
	if err != nil {
		t.Fatalf("RenderInstructions: %v", err)
	}
	if v != 1 {
		t.Fatalf("expected version 1, got %d", v)
	}
	expected := "You are @my-agent in Acme Corp. Admins: @alice, @bob. Unknown: {unknown_placeholder}."
	if rendered != expected {
		t.Fatalf("rendered rules mismatch:\ngot:  %q\nwant: %q", rendered, expected)
	}

	// 6. InstructionsForActor
	agentInstr, err := svc.InstructionsForActor(ctx, agentActor)
	if err != nil {
		t.Fatalf("InstructionsForActor for agent: %v", err)
	}
	if agentInstr != expected {
		t.Fatalf("unexpected agent instructions: %q", agentInstr)
	}

	humanInstr, err := svc.InstructionsForActor(ctx, adminActor)
	if err != nil {
		t.Fatalf("InstructionsForActor for human: %v", err)
	}
	if humanInstr != "" {
		t.Fatalf("expected empty instructions for human actor, got %q", humanInstr)
	}
}
