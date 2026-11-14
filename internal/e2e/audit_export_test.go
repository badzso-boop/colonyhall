// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func auditPage(e *env, token, query string) apispec.AuditPage {
	e.t.Helper()
	var page apispec.AuditPage
	if st := e.do(http.MethodGet, "/api/v1/admin/audit?"+query, token, nil, &page); st != http.StatusOK {
		e.t.Fatalf("audit %q: %d", query, st)
	}
	return page
}

func TestAuditFiltersAndExport(t *testing.T) {
	e := newEnv(t)
	admin := server.DevAdminToken
	e.send(admin, e.generalID(), map[string]any{"body": "első"})
	_, memberToken := e.createActor("human", "anna", "member")
	if err := e.app.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}

	all := auditPage(e, admin, "limit=200")
	if len(all.Entries) == 0 {
		t.Fatal("empty audit log")
	}
	action := all.Entries[0].Action
	for _, row := range auditPage(e, admin, "action="+url.QueryEscape(action)).Entries {
		if row.Action != action {
			t.Fatalf("action filter let through %q", row.Action)
		}
	}
	// A time window in the far past selects nothing.
	if got := auditPage(e, admin, "to=2000-01-01T00:00:00Z").Entries; len(got) != 0 {
		t.Fatalf("time filter returned %d rows", len(got))
	}
	var bad struct{}
	if st := e.do(http.MethodGet, "/api/v1/admin/audit?from=yesterday", admin, nil, &bad); st != http.StatusBadRequest {
		t.Fatalf("bad from: %d", st)
	}

	var actions apispec.AuditActions
	if st := e.do(http.MethodGet, "/api/v1/admin/audit/actions", admin, nil, &actions); st != http.StatusOK || len(actions.Actions) == 0 {
		t.Fatalf("actions: %d %v", st, actions)
	}

	get := func(token, query string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, e.srv.URL+"/api/v1/admin/audit/export?"+query, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(raw)
	}
	if st, _ := get(memberToken, ""); st != http.StatusForbidden {
		t.Fatalf("member export: %d", st)
	}
	st, body := get(admin, "action="+url.QueryEscape(action))
	if st != http.StatusOK {
		t.Fatalf("export: %d", st)
	}
	if !strings.HasPrefix(body, "\xef\xbb\xbfseq,at,actor,action,target_type,target_id,payload\n") {
		t.Fatalf("no BOM and header: %q", body[:min(len(body), 80)])
	}
	if !strings.Contains(body, action) {
		t.Fatalf("export misses %q: %s", action, body)
	}

	// The export itself is on the record.
	if err := e.app.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := auditPage(e, admin, "action=audit.exported").Entries; len(got) != 1 {
		t.Fatalf("audit.exported rows: %d", len(got))
	}
}
