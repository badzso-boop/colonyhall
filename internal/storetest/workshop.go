// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package storetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"colonyhall/internal/modules/secrets"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/db"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/platform/secretbox"
)

// testSecrets: a sealed value round-trips, is replaced and deleted, and a
// second workspace never sees it (round 56a).
func testSecrets(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Secrets
	sec := secrets.Secret{ID: gen.New(), WorkspaceID: f.ws.ID, Kind: secrets.KindConnectorToken, Name: "jira",
		Sealed: secretbox.Sealed{KeyID: "k1", Nonce: []byte{1, 2}, Ciphertext: []byte{3, 4}}, Hint: "wxyz",
		CreatedBy: f.human.ID, CreatedAt: t0}
	must(t, st.Insert(ctx, sec))
	got, err := st.Get(ctx, f.ws.ID, sec.ID)
	must(t, err)
	if got.Name != "jira" || got.Hint != "wxyz" || !bytes.Equal(got.Sealed.Ciphertext, []byte{3, 4}) || got.Sealed.KeyID != "k1" {
		t.Fatalf("get: %+v", got)
	}
	must(t, st.Replace(ctx, f.ws.ID, sec.ID, secretbox.Sealed{KeyID: "k2", Nonce: []byte{5}, Ciphertext: []byte{6}}, "abcd", t0.Add(time.Hour)))
	got, err = st.Get(ctx, f.ws.ID, sec.ID)
	must(t, err)
	if got.Sealed.KeyID != "k2" || got.Hint != "abcd" || got.RotatedAt == nil {
		t.Fatalf("replace: %+v", got)
	}
	stale, err := st.ListForRotation(b.System(ctx), "k3")
	must(t, err)
	if len(stale) != 1 || stale[0].ID != sec.ID {
		t.Fatalf("for rotation: %+v", stale)
	}
	if none, _ := st.ListForRotation(b.System(ctx), "k2"); len(none) != 0 {
		t.Fatalf("already on the active key: %+v", none)
	}
	must(t, st.Reseal(ctx, f.ws.ID, sec.ID, secretbox.Sealed{KeyID: "k3", Nonce: []byte{7}, Ciphertext: []byte{8}}))
	if got, _ := st.Get(ctx, f.ws.ID, sec.ID); got.Sealed.KeyID != "k3" || got.Hint != "abcd" {
		t.Fatalf("reseal: %+v", got)
	}
	if _, err := st.Get(ctx, gen.New(), sec.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("another workspace id: %v", err)
	}
	must(t, st.Delete(ctx, f.ws.ID, sec.ID, t0.Add(2*time.Hour)))
	if _, err := st.Get(ctx, f.ws.ID, sec.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("deleted secret still found: %v", err)
	}
	if err := st.Delete(ctx, f.ws.ID, sec.ID, t0); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

// testWorkshop: connectors (unique live slug, tools cache, update, soft
// delete), grants (one live grant per target, revoke), the call log.
func testWorkshop(t *testing.T, b Backend) {
	f := setup(t, b)
	ctx := db.WithWorkspace(context.Background(), f.ws.ID)
	st := b.Workshop
	c := workshop.Connector{ID: gen.New(), WorkspaceID: f.ws.ID, Slug: "jira", Title: "Jira", Kind: workshop.KindMCP,
		BaseURL: "https://mcp.example.com/mcp", Auth: workshop.AuthNone, RiskOverrides: map[string]string{},
		State: "new", CreatedBy: f.human.ID, CreatedAt: t0, UpdatedAt: t0}
	must(t, st.InsertConnector(ctx, c))
	dup := c
	dup.ID = gen.New()
	if err := st.InsertConnector(ctx, dup); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("duplicate live slug: %v", err)
	}
	tools := []workshop.ToolInfo{{Name: "search", Description: "Search issues", InputSchema: []byte(`{"type":"object"}`), Risk: workshop.RiskRead}}
	must(t, st.SetConnectorTools(ctx, f.ws.ID, c.ID, tools, t0.Add(time.Minute), "ok", ""))
	c.Title, c.ApprovalOff, c.RiskOverrides, c.UpdatedAt = "Jira Cloud", true, map[string]string{"search": "external"}, t0.Add(2*time.Minute)
	must(t, st.UpdateConnector(ctx, c))
	got, err := st.GetConnector(ctx, f.ws.ID, c.ID)
	must(t, err)
	if got.Title != "Jira Cloud" || !got.ApprovalOff || got.RiskOverrides["search"] != "external" || got.State != "ok" ||
		len(got.Tools) != 1 || got.Tools[0].Name != "search" || !sameJSON(got.Tools[0].InputSchema, `{"type":"object"}`) ||
		got.ToolsCachedAt == nil {
		t.Fatalf("get: %+v", got)
	}
	list, err := st.ListConnectors(ctx, f.ws.ID)
	must(t, err)
	if len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	must(t, st.DeleteConnector(ctx, f.ws.ID, c.ID, t0.Add(time.Hour)))
	if _, err := st.GetConnector(ctx, f.ws.ID, c.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("deleted connector: %v", err)
	}
	must(t, st.InsertConnector(ctx, dup)) // the slug is free again

	g := workshop.Grant{ID: gen.New(), WorkspaceID: f.ws.ID, AgentID: f.agent.ID, TargetKind: workshop.TargetConnector,
		TargetID: dup.ID, GrantedBy: f.human.ID, CreatedAt: t0}
	must(t, st.InsertGrant(ctx, g))
	if err := st.InsertGrant(ctx, workshop.Grant{ID: gen.New(), WorkspaceID: f.ws.ID, AgentID: f.agent.ID,
		TargetKind: workshop.TargetConnector, TargetID: dup.ID, GrantedBy: f.human.ID, CreatedAt: t0}); !errors.Is(err, db.ErrConflict) {
		t.Fatalf("second live grant: %v", err)
	}
	w := workshop.Grant{ID: gen.New(), WorkspaceID: f.ws.ID, AgentID: f.agent.ID, TargetKind: workshop.TargetWorkshop,
		GrantedBy: f.human.ID, CreatedAt: t0.Add(time.Second)}
	must(t, st.InsertGrant(ctx, w))
	mine, err := st.GrantsForAgent(ctx, f.ws.ID, f.agent.ID)
	must(t, err)
	if len(mine) != 2 || mine[1].TargetID != ids.Nil {
		t.Fatalf("grants for agent: %+v", mine)
	}
	must(t, st.RevokeGrantsForTarget(ctx, f.ws.ID, workshop.TargetConnector, dup.ID, t0.Add(time.Hour)))
	all, err := st.ListGrants(ctx, f.ws.ID)
	must(t, err)
	if len(all) != 1 || all[0].ID != w.ID {
		t.Fatalf("after revoking the connector's grants: %+v", all)
	}
	must(t, st.RevokeGrant(ctx, f.ws.ID, w.ID, t0.Add(time.Hour)))
	if err := st.RevokeGrant(ctx, f.ws.ID, w.ID, t0.Add(time.Hour)); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}

	for i := 0; i < 3; i++ {
		must(t, st.InsertToolCall(ctx, workshop.ToolCall{ID: gen.New(), WorkspaceID: f.ws.ID, ActorID: f.agent.ID,
			TargetKind: workshop.TargetConnector, TargetID: dup.ID, Tool: "search", Risk: workshop.RiskRead,
			StartedAt: t0.Add(time.Duration(i) * time.Minute), DurationMS: 5, InBytes: 10, OutBytes: 20, Outcome: workshop.OutcomeOK}))
	}
	calls, err := st.ListToolCalls(ctx, f.ws.ID, nil, nil, 2)
	must(t, err)
	if len(calls) != 2 || !calls[0].StartedAt.Equal(t0.Add(2*time.Minute)) || calls[0].OutBytes != 20 {
		t.Fatalf("calls: %+v", calls)
	}
	before := calls[1].StartedAt
	older, err := st.ListToolCalls(ctx, f.ws.ID, &dup.ID, &before, 10)
	must(t, err)
	if len(older) != 1 || !older[0].StartedAt.Equal(t0) {
		t.Fatalf("older calls: %+v", older)
	}
}

// sameJSON compares JSON by value (Postgres jsonb reformats it).
func sameJSON(a []byte, b string) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
