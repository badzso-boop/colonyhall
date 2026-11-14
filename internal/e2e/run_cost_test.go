// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/modules/runs"
	"colonyhall/internal/server"
)

func (e *env) addRun(agent identity.Actor, model, client, status string, in, out int64, at time.Time) {
	e.t.Helper()
	err := e.app.Runs.Store.InsertRun(context.Background(), runs.Run{ID: e.app.Runs.IDs.New(), WorkspaceID: agent.WorkspaceID, AgentID: agent.ID, Phase: "direct",
		Client: client, Model: model, SessionID: "s", InputTokens: in, OutputTokens: out, DurationMS: 1000, Status: status, CreatedAt: at})
	if err != nil {
		e.t.Fatal(err)
	}
}

func TestPricesAndEstimatedCost(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	_, memberTok := e.createUser("anna", "Anna")
	agent, err := e.app.Identity.Authenticate(ctx, server.DevAgentToken)
	if err != nil {
		t.Fatal(err)
	}
	now := e.clock.Now()
	price := map[string]any{"model": "claude-*", "input_per_mtok": 10.0, "output_per_mtok": 20.0, "cache_read_per_mtok": 1.0, "cache_write_per_mtok": 2.0, "valid_from": now.Add(-48 * time.Hour)}

	// Only admins see and change the table and the rate.
	for _, tok := range []string{memberTok, server.DevAgentToken} {
		if s := e.do("POST", "/api/v1/settings/prices", tok, price, nil); s != http.StatusForbidden {
			t.Fatalf("add price: %d", s)
		}
		if s := e.do("GET", "/api/v1/settings/prices", tok, nil, nil); s != http.StatusForbidden {
			t.Fatalf("list prices: %d", s)
		}
		if s := e.do("PUT", "/api/v1/settings/currency", tok, map[string]any{"usd_huf": 360.0}, nil); s != http.StatusForbidden {
			t.Fatalf("set rate: %d", s)
		}
	}
	// A run before any price: no cost, counted as unpriced.
	e.addRun(agent, "gemini-3", "antigravity", "ok", 1000, 1000, now.Add(-time.Hour))

	if s := e.do("POST", "/api/v1/settings/prices", server.DevAdminToken, price, nil); s != http.StatusCreated {
		t.Fatalf("add price: %d", s)
	}
	if s := e.do("POST", "/api/v1/settings/prices", server.DevAdminToken, map[string]any{"model": "a*b", "input_per_mtok": 1.0, "output_per_mtok": 1.0, "cache_read_per_mtok": 1.0, "cache_write_per_mtok": 1.0}, nil); s != http.StatusBadRequest {
		t.Fatalf("a bad pattern: %d", s)
	}
	// A subscription run and an API run at the old price.
	e.addRun(agent, "claude-opus", "claude-code", "ok", 1_000_000, 0, now.Add(-2*time.Hour))
	e.addRun(agent, "claude-opus", "api", "error", 0, 1_000_000, now.Add(-90*time.Minute))
	// The price changes: an old run keeps the old one, a new run gets the new one.
	if s := e.do("POST", "/api/v1/settings/prices", server.DevAdminToken, map[string]any{"model": "claude-*", "input_per_mtok": 100.0, "output_per_mtok": 200.0, "cache_read_per_mtok": 1.0, "cache_write_per_mtok": 2.0, "valid_from": now.Add(-time.Hour)}, nil); s != http.StatusCreated {
		t.Fatal("second price")
	}
	e.addRun(agent, "claude-opus", "claude-code", "ok", 1_000_000, 0, now.Add(-time.Minute))

	var list apispec.PriceList
	e.do("GET", "/api/v1/settings/prices", server.DevAdminToken, nil, &list)
	if len(list.Prices) != 2 {
		t.Fatalf("old rows must stay: %+v", list.Prices)
	}

	var page apispec.RunList
	if s := e.do("GET", "/api/v1/runs?limit=10", server.DevAdminToken, nil, &page); s != http.StatusOK || len(page.Runs) != 4 {
		t.Fatalf("runs: %d %d", s, len(page.Runs))
	}
	want := []float64{100, -1, 20, 10} // newest first; -1 means no price
	for i, r := range page.Runs {
		switch {
		case want[i] < 0 && r.EstimatedUsd != nil:
			t.Errorf("run %d: want no cost, got %v", i, *r.EstimatedUsd)
		case want[i] >= 0 && (r.EstimatedUsd == nil || math.Abs(*r.EstimatedUsd-want[i]) > 1e-9):
			t.Errorf("run %d: cost %v, want %v", i, r.EstimatedUsd, want[i])
		}
	}
	if page.Runs[0].Billing == nil || *page.Runs[0].Billing != "subscription" || *page.Runs[2].Billing != "api" {
		t.Errorf("billing: %v %v", page.Runs[0].Billing, page.Runs[2].Billing)
	}

	// Filters and paging.
	var failed apispec.RunList
	e.do("GET", "/api/v1/runs?status=error", server.DevAdminToken, nil, &failed)
	if len(failed.Runs) != 1 || failed.Runs[0].Client != "api" {
		t.Fatalf("status filter: %+v", failed.Runs)
	}
	var first, second apispec.RunList
	e.do("GET", "/api/v1/runs?limit=3", server.DevAdminToken, nil, &first)
	if len(first.Runs) != 3 || first.NextCursor == nil {
		t.Fatalf("first page: %d %v", len(first.Runs), first.NextCursor)
	}
	e.do("GET", fmt.Sprintf("/api/v1/runs?limit=3&cursor=%s", *first.NextCursor), server.DevAdminToken, nil, &second)
	if len(second.Runs) != 1 || second.NextCursor != nil || second.Runs[0].Id == first.Runs[2].Id {
		t.Fatalf("second page: %+v", second)
	}
	if s := e.do("GET", "/api/v1/runs?cursor=nonsense", server.DevAdminToken, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("a bad cursor: %d", s)
	}
	var range1 apispec.RunList
	e.do("GET", "/api/v1/runs?to="+now.Add(-100*time.Minute).UTC().Format(time.RFC3339), server.DevAdminToken, nil, &range1)
	if len(range1.Runs) != 1 {
		t.Fatalf("to filter: %d", len(range1.Runs))
	}

	// Usage: the split by billing and grouping by model.
	var usage apispec.UsageResponse
	if s := e.do("GET", "/api/v1/usage?days=7&group_by=model", server.DevAdminToken, nil, &usage); s != http.StatusOK || usage.Totals == nil {
		t.Fatalf("usage: %d %+v", s, usage)
	}
	if tot := usage.Totals; tot.RunCount != 4 || tot.ErrorCount != 1 || tot.UnpricedRuns != 1 || math.Abs(tot.SubscriptionUsd-110) > 1e-9 || math.Abs(tot.ApiUsd-20) > 1e-9 {
		t.Fatalf("totals: %+v", tot)
	}
	models := map[string]bool{}
	for _, g := range *usage.Groups {
		models[g.Key] = true
	}
	if !models["claude-opus"] || !models["gemini-3"] {
		t.Fatalf("group by model: %+v", *usage.Groups)
	}
	if s := e.do("GET", "/api/v1/usage?group_by=nonsense", server.DevAdminToken, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("a bad group: %d", s)
	}

	// The rate is empty until it is set.
	var cur apispec.CurrencySettings
	e.do("GET", "/api/v1/settings/currency", server.DevAdminToken, nil, &cur)
	if cur.UsdHuf != nil {
		t.Fatalf("rate before setting: %v", *cur.UsdHuf)
	}
	if s := e.do("PUT", "/api/v1/settings/currency", server.DevAdminToken, map[string]any{"usd_huf": -1.0}, nil); s != http.StatusBadRequest {
		t.Fatalf("negative rate: %d", s)
	}
	if s := e.do("PUT", "/api/v1/settings/currency", server.DevAdminToken, map[string]any{"usd_huf": 365.5}, &cur); s != http.StatusOK || cur.UsdHuf == nil || *cur.UsdHuf != 365.5 {
		t.Fatalf("set rate: %d %+v", s, cur)
	}
}
