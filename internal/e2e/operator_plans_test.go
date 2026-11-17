// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"net/http"
	"os"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
	"colonyhall/internal/storetest"
)

// Round 51b (docs/24, 7. pont): the operator's plans. A plan limits the
// workspace's storage; the owner may lower it, never raise it; a plan
// below the usage needs a confirmation and leaves the workspace full.

func TestOperatorPlans(t *testing.T) { runOperatorPlans(t, nil) }

// TestPostgresOperatorPlans: the same on Postgres with row-level security,
// as the application role (the overview function, the operator scope, the
// storage row's plan columns).
func TestPostgresOperatorPlans(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	app := storetest.AppRoleDatabase(t, url)
	runOperatorPlans(t, func(o *server.Options) {
		o.Store, o.DatabaseURL, o.IDs, o.FilesDir = "postgres", app, ids.V7{}, t.TempDir()
	})
}

func runOperatorPlans(t *testing.T, change func(*server.Options)) {
	acme, _, secret := opEnv(t, change)
	op := acme.on("admin." + baseDomain).client()
	op.operatorLogin(secret)
	owner := server.DevAdminToken

	var cust apispec.CustomerList
	if s := op.do("GET", "/api/v1/operator/workspaces", nil, &cust); s != http.StatusOK || len(cust.Customers) < 2 {
		t.Fatalf("customers: %d %+v", s, cust)
	}
	acmeID := ""
	for _, c := range cust.Customers {
		if c.Slug == "acme" {
			acmeID = c.Id.String()
		}
	}

	// Some usage: a file of 100 bytes.
	if s, _ := uploadMultipart(t, acme, acme.generalID(), "data.bin", make([]byte, 100), owner); s != http.StatusCreated {
		t.Fatalf("upload: %d", s)
	}

	var big, small apispec.Plan
	if s := op.do("POST", "/api/v1/operator/plans", map[string]any{"name": "Közepes", "storage_bytes": 10_000, "cpu_cores": 4, "memory_bytes": 4 << 30, "ext_calls_month": 5000}, &big); s != http.StatusCreated ||
		big.ExtCallsMonth == nil || *big.ExtCallsMonth != 5000 {
		t.Fatalf("create plan: %d", s)
	}
	if s := op.do("POST", "/api/v1/operator/plans", map[string]any{"name": "Közepes", "storage_bytes": 1, "cpu_cores": 1, "memory_bytes": 1}, nil); s != http.StatusConflict {
		t.Fatalf("a taken name: %d", s)
	}
	if s := op.do("POST", "/api/v1/operator/plans", map[string]any{"name": "Pici", "storage_bytes": 50, "cpu_cores": 2, "memory_bytes": 2 << 30}, &small); s != http.StatusCreated {
		t.Fatalf("create small plan: %d", s)
	}
	if s := op.do("POST", "/api/v1/operator/plans", map[string]any{"name": "Rossz", "storage_bytes": 1, "cpu_cores": 0, "memory_bytes": 1}, nil); s != http.StatusBadRequest {
		t.Fatalf("an invalid plan: %d", s)
	}

	// Assign the big plan: the owner sees it, and the limit is the plan.
	var c apispec.Customer
	if s := op.do("PUT", "/api/v1/operator/workspaces/"+acmeID+"/plan", map[string]any{"plan_id": big.Id}, &c); s != http.StatusOK ||
		c.PlanId == nil || *c.PlanId != big.Id || c.Storage.QuotaBytes != 10_000 {
		t.Fatalf("assign: %d %+v", s, c)
	}
	var st apispec.WorkspaceStorage
	if s := acme.do("GET", "/api/v1/workspace/storage", owner, nil, &st); s != http.StatusOK || st.Plan == nil ||
		st.Plan.Name != "Közepes" || st.Plan.CpuCores != 4 || st.LimitBytes != 10_000 || st.QuotaBytes != 0 ||
		st.Plan.ExtCallsMonth == nil || *st.Plan.ExtCallsMonth != 5000 {
		t.Fatalf("the owner's storage with a plan: %d %+v %+v", s, st, st.Plan)
	}
	// The owner may lower it, never raise it.
	if s := acme.do("PUT", "/api/v1/workspace/storage/quota", owner, map[string]any{"quota_bytes": 20_000}, nil); s != http.StatusUnprocessableEntity {
		t.Fatalf("a quota above the plan: %d", s)
	}
	if s := acme.do("PUT", "/api/v1/workspace/storage/quota", owner, map[string]any{"quota_bytes": 5_000}, &st); s != http.StatusOK || st.LimitBytes != 5_000 {
		t.Fatalf("a quota below the plan: %d %+v", s, st)
	}
	acme.do("PUT", "/api/v1/workspace/storage/quota", owner, map[string]any{"quota_bytes": 0}, nil)

	// The small plan is below the usage: the check says so, the assignment
	// needs a confirmation, and then the workspace is full.
	var chk apispec.PlanCheck
	if s := op.do("GET", "/api/v1/operator/workspaces/"+acmeID+"/plan-check?plan_id="+small.Id.String(), nil, &chk); s != http.StatusOK ||
		chk.Fits || chk.StateAfter != apispec.StorageStateFull || chk.NewQuotaBytes != 50 || chk.UsedBytes < 100 {
		t.Fatalf("plan check: %d %+v", s, chk)
	}
	if s := op.do("PUT", "/api/v1/operator/workspaces/"+acmeID+"/plan", map[string]any{"plan_id": small.Id}, nil); s != http.StatusConflict {
		t.Fatalf("a plan below the usage without confirmation: %d", s)
	}
	if s := acme.do("GET", "/api/v1/workspace/storage", owner, nil, &st); s != http.StatusOK || st.Plan == nil || st.Plan.Name != "Közepes" {
		t.Fatalf("the refused change changed the plan: %+v", st.Plan)
	}
	if s := op.do("PUT", "/api/v1/operator/workspaces/"+acmeID+"/plan", map[string]any{"plan_id": small.Id, "confirm_over_quota": true}, &c); s != http.StatusOK ||
		c.Storage.State != apispec.CustomerStorageStateFull {
		t.Fatalf("a confirmed small plan: %d %+v", s, c.Storage)
	}
	if s := acme.do("GET", "/api/v1/workspace/storage", owner, nil, &st); st.State != apispec.StorageStateFull || st.LimitBytes != 50 {
		t.Fatalf("the workspace is not full on the small plan: %d %+v", s, st)
	}
	if s, _ := uploadMultipart(t, acme, acme.generalID(), "more.bin", []byte("x"), owner); s != http.StatusInsufficientStorage {
		t.Fatalf("an upload on a full plan: %d", s)
	}

	// Editing a plan reaches its workspaces; growing it is free, shrinking
	// a workspace into full needs a confirmation.
	var p apispec.Plan
	if s := op.do("PATCH", "/api/v1/operator/plans/"+small.Id.String(), map[string]any{"storage_bytes": 100_000}, &p); s != http.StatusOK || p.Workspaces != 1 {
		t.Fatalf("grow the plan: %d %+v", s, p)
	}
	if s := acme.do("GET", "/api/v1/workspace/storage", owner, nil, &st); st.State != apispec.StorageStateOk || st.LimitBytes != 100_000 {
		t.Fatalf("the plan's new storage did not reach the workspace: %d %+v", s, st)
	}
	if s := op.do("PATCH", "/api/v1/operator/plans/"+small.Id.String(), map[string]any{"storage_bytes": 10}, nil); s != http.StatusConflict {
		t.Fatalf("shrink a plan below a workspace's usage without confirmation: %d", s)
	}
	if s := acme.do("GET", "/api/v1/workspace/storage", owner, nil, &st); st.LimitBytes != 100_000 {
		t.Fatalf("the refused edit changed the workspace: %d %+v", s, st)
	}

	// An archived plan cannot be assigned; the workspace keeps its plan.
	if s := op.do("PATCH", "/api/v1/operator/plans/"+big.Id.String(), map[string]any{"archived": true}, nil); s != http.StatusOK {
		t.Fatalf("archive: %d", s)
	}
	if s := op.do("PUT", "/api/v1/operator/workspaces/"+acmeID+"/plan", map[string]any{"plan_id": big.Id}, nil); s != http.StatusConflict {
		t.Fatalf("assign an archived plan: %d", s)
	}
	var plans apispec.PlanList
	if s := op.do("GET", "/api/v1/operator/plans", nil, &plans); s != http.StatusOK || len(plans.Plans) != 2 ||
		plans.Plans[0].Id != small.Id || plans.Plans[1].ArchivedAt == nil {
		t.Fatalf("plans: %d %+v", s, plans)
	}

	// No plan: only the owner's quota again.
	var none apispec.Customer
	if s := op.do("PUT", "/api/v1/operator/workspaces/"+acmeID+"/plan", map[string]any{}, &none); s != http.StatusOK || none.PlanId != nil {
		t.Fatalf("remove the plan: %d %+v", s, none)
	}
	var plain apispec.WorkspaceStorage
	if s := acme.do("GET", "/api/v1/workspace/storage", owner, nil, &plain); plain.Plan != nil || plain.LimitBytes != 0 {
		t.Fatalf("without a plan: %d %+v", s, plain)
	}
}
