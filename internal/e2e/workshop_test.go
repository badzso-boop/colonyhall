// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/platform/egress"
	"colonyhall/internal/platform/secretbox"
	"colonyhall/internal/server"
)

const jiraSecret = "jira-token-0123456789abcd"

// fakeJira is an external MCP server (round 56a's connector target) with a
// read tool and a write tool; it records every Authorization header.
type fakeJira struct {
	url  string
	mu   sync.Mutex
	auth []string
}

func (f *fakeJira) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.auth...)
}

func newFakeJira(t *testing.T) *fakeJira {
	t.Helper()
	f := &fakeJira{}
	type in struct {
		Q string `json:"q"`
	}
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		s := mcp.NewServer(&mcp.Implementation{Name: "fake-jira"}, nil)
		mcp.AddTool(s, &mcp.Tool{Name: "search", Description: "Search issues", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true}},
			func(_ context.Context, _ *mcp.CallToolRequest, a in) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "PROJ-1 for " + a.Q}}}, nil, nil
			})
		mcp.AddTool(s, &mcp.Tool{Name: "create_issue", Description: "Create an issue"},
			func(_ context.Context, _ *mcp.CallToolRequest, a in) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "created"}}}, nil, nil
			})
		return s
	}, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.mu.Unlock()
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	f.url = srv.URL + "/mcp"
	return f
}

func workshopEnv(t *testing.T, withKeys bool) *env {
	t.Helper()
	return newEnvWith(t, func(o *server.Options) {
		if withKeys {
			k, err := secretbox.NewKey("k1")
			if err != nil {
				t.Fatal(err)
			}
			if o.SecretKeys, err = secretbox.Parse(k); err != nil {
				t.Fatal(err)
			}
		}
		// The fake server listens on loopback: the operator's allow rule.
		o.EgressPolicy, _ = egress.ParsePolicy("127.0.0.1")
	})
}

func toolNames(t *testing.T, e *env, token string) []string {
	t.Helper()
	ctx := context.Background()
	s, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", token, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var out []string
	for tl, err := range s.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, tl.Name)
	}
	return out
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestWorkshopConnectorEndToEnd: an admin adds an MCP connector with a
// secret; an agent sees its tools only after a grant, calls the read tool
// through the gateway without ever seeing the secret, and the write tool
// needs an approval; the call log has no content.
func TestWorkshopConnectorEndToEnd(t *testing.T) {
	e := workshopEnv(t, true)
	jira := newFakeJira(t)
	admin := server.DevAdminToken
	_, memberTok := e.createUser("bela", "Béla")
	agentID, agentTok := e.newAgent("jirabot", "Jira bot", nil, nil)

	body := map[string]any{"slug": "jira", "title": "Jira", "kind": "mcp", "base_url": jira.url, "auth": "bearer", "secret": jiraSecret}
	if s := e.do("POST", "/api/v1/workshop/connectors", memberTok, body, nil); s != http.StatusForbidden {
		t.Fatalf("a member created a connector: %d", s)
	}
	var c apispec.Connector
	if s := e.do("POST", "/api/v1/workshop/connectors", admin, body, &c); s != http.StatusCreated {
		t.Fatalf("create connector: %d", s)
	}
	if c.State != "ok" || len(c.Tools) != 2 || !c.HasSecret || c.SecretHint == nil || *c.SecretHint != "abcd" {
		t.Fatalf("connector: %+v", c)
	}
	if s := e.do("POST", "/api/v1/workshop/connectors", admin, body, nil); s != http.StatusConflict {
		t.Fatalf("same slug twice: %d", s)
	}

	// No grant yet: the agent does not see the tools.
	if names := toolNames(t, e, agentTok); has(names, "jira__search") {
		t.Fatalf("tools without a grant: %v", names)
	}
	var g apispec.ToolGrant
	if s := e.do("POST", "/api/v1/workshop/grants", admin, map[string]any{"agent_id": agentID, "target_kind": "connector", "target_id": c.Id}, &g); s != http.StatusOK {
		t.Fatalf("grant: %d", s)
	}
	names := toolNames(t, e, agentTok)
	if !has(names, "jira__search") || !has(names, "jira__create_issue") || !has(names, "send") {
		t.Fatalf("tools after the grant: %v", names)
	}
	out, isErr := e.mcpCall(agentTok, "jira__search", map[string]any{"q": "norbi"})
	if isErr || out != "PROJ-1 for norbi" {
		t.Fatalf("read call: %q %v", out, isErr)
	}
	for _, h := range jira.seen() {
		if h != "Bearer "+jiraSecret {
			t.Fatalf("a request to the connector without its secret: %q", h)
		}
	}
	// The write tool asks for approval; outside a conversation it is refused.
	out, isErr = e.mcpCall(agentTok, "jira__create_issue", map[string]any{"q": "x"})
	if !isErr || !strings.Contains(out, "approval") {
		t.Fatalf("write call without a conversation: %q %v", out, isErr)
	}

	// Another agent without a grant cannot call it even by name.
	_, otherTok := e.newAgent("other", "Other", nil, nil)
	{
		ctx := context.Background()
		s, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", otherTok, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "jira__search", Arguments: map[string]any{"q": "x"}})
		_ = s.Close()
		if err == nil && !res.IsError {
			t.Fatal("an agent without a grant called the tool")
		}
	}

	var calls apispec.ToolCallList
	if s := e.do("GET", "/api/v1/workshop/calls", admin, nil, &calls); s != http.StatusOK || len(calls.Calls) != 2 {
		t.Fatalf("call log: %d %+v", s, calls)
	}
	if calls.Calls[0].Outcome != "denied" || calls.Calls[1].Outcome != "ok" || calls.Calls[1].Risk != "read" {
		t.Fatalf("call log entries: %+v", calls.Calls)
	}

	// The secret never appears in any answer.
	for _, path := range []string{"/api/v1/workshop/connectors", "/api/v1/workshop/connectors/" + c.Id.String(), "/api/v1/workshop/calls", "/api/v1/workshop/grants", "/api/v1/admin/audit"} {
		var raw json.RawMessage
		e.do("GET", path, admin, nil, &raw)
		if strings.Contains(string(raw), jiraSecret) {
			t.Fatalf("%s shows the secret", path)
		}
	}

	// Only the owner switches the approval off. (The dev seed's admin is
	// the owner; a second admin is refused.)
	annaID, admin2 := e.createUser("anna", "Anna")
	if s := e.do("PUT", "/api/v1/actors/"+annaID+"/role", admin, map[string]any{"role": "admin"}, nil); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("make anna an admin: %d", s)
	}
	if s := e.do("PUT", "/api/v1/workshop/connectors/"+c.Id.String()+"/approval", admin2, map[string]any{"approval_off": true}, nil); s != http.StatusForbidden {
		t.Fatalf("an admin switched the approval off: %d", s)
	}
	if s := e.do("PUT", "/api/v1/workshop/connectors/"+c.Id.String()+"/approval", admin, map[string]any{"approval_off": true}, &c); s != http.StatusOK || !c.ApprovalOff {
		t.Fatalf("owner switches the approval off: %d %+v", s, c)
	}
	if out, isErr := e.mcpCall(agentTok, "jira__create_issue", map[string]any{"q": "x"}); isErr || out != "created" {
		t.Fatalf("write call with the approval off: %q %v", out, isErr)
	}

	// Revoking the grant takes the tools away.
	if s := e.do("DELETE", "/api/v1/workshop/grants/"+g.Id.String(), admin, nil, nil); s != http.StatusNoContent {
		t.Fatalf("revoke: %d", s)
	}
	if names := toolNames(t, e, agentTok); has(names, "jira__search") {
		t.Fatalf("tools after revoke: %v", names)
	}
	if s := e.do("DELETE", "/api/v1/workshop/connectors/"+c.Id.String(), admin, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete: %d", s)
	}
}

// Without COLONY_SECRET_KEYS a connector cannot carry a secret, and the
// list says so.
func TestWorkshopWithoutSecretKeys(t *testing.T) {
	e := workshopEnv(t, false)
	jira := newFakeJira(t)
	var list apispec.ConnectorList
	if s := e.do("GET", "/api/v1/workshop/connectors", server.DevAdminToken, nil, &list); s != http.StatusOK || list.SecretsAvailable {
		t.Fatalf("list: %d %+v", s, list)
	}
	body := map[string]any{"slug": "jira", "title": "Jira", "kind": "mcp", "base_url": jira.url, "auth": "bearer", "secret": "x"}
	if s := e.do("POST", "/api/v1/workshop/connectors", server.DevAdminToken, body, nil); s != http.StatusBadRequest {
		t.Fatalf("a secret without keys: %d", s)
	}
	delete(body, "secret")
	body["auth"] = "none"
	if s := e.do("POST", "/api/v1/workshop/connectors", server.DevAdminToken, body, nil); s != http.StatusCreated {
		t.Fatalf("a connector without a secret: %d", s)
	}
}

// Without an allow rule a connector to an internal address is not
// reached: the connector is created but its state is error.
func TestWorkshopInternalAddressRefused(t *testing.T) {
	e := newEnv(t)
	jira := newFakeJira(t)
	var c apispec.Connector
	body := map[string]any{"slug": "jira", "title": "Jira", "kind": "mcp", "base_url": jira.url}
	if s := e.do("POST", "/api/v1/workshop/connectors", server.DevAdminToken, body, &c); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	if c.State != "error" || len(jira.seen()) != 0 {
		t.Fatalf("an internal address was reached: %+v seen=%v", c, jira.seen())
	}
}
