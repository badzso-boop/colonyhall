// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/modules/wiki"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/server"
)

// withHeaders puts the guard's conversation headers on every MCP request.
type withHeaders struct {
	h    map[string]string
	next http.RoundTripper
}

func (w withHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range w.h {
		r.Header.Set(k, v)
	}
	return w.next.RoundTrip(r)
}

func callIn(t *testing.T, s *mcp.ClientSession, tool string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

var approvalRef = regexp.MustCompile(`\[approval:([0-9a-f-]{36})\]`)

// TestBuilderBuildsTheGifExtension is docs/25's 56d: the Builder (the fake
// agent with the Workshop grant, owned by the owner) creates the GIF
// extension, tests it, asks for publishing; a person allows it on the card
// and it goes live. The build log is in the wiki; the extension exports
// and imports.
func TestBuilderBuildsTheGifExtension(t *testing.T) {
	fetch := &fakeFetcher{}
	e := extEnv(t, fetch, workshop.Limits{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = e.app.Run(ctx) }()
	admin := server.DevAdminToken
	var me struct {
		Actor apispec.Actor `json:"actor"`
	}
	e.do("GET", "/api/v1/auth/session", admin, nil, &me)

	if s := e.do("POST", "/api/v1/workshop/connectors", admin, map[string]any{"slug": "tenor", "title": "Tenor", "kind": "http",
		"base_url": "https://tenor.example.com", "auth": "header", "header_name": "X-Api-Key", "secret": testExtSecret}, nil); s != http.StatusCreated {
		t.Fatalf("connector: %d", s)
	}
	builderID, builderTok := e.newAgent("builder", "Építő", nil, nil)
	_, otherTok := e.newAgent("other", "Other", nil, nil)
	if s := e.do("PUT", "/api/v1/agents/"+builderID+"/owner", admin, map[string]any{"owner_id": me.Actor.Id}, nil); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("owner: %d", s)
	}
	if names := toolNames(t, e, builderTok); has(names, "extension_draft") {
		t.Fatal("meta-tools without the Workshop grant")
	}
	if s := e.do("POST", "/api/v1/workshop/grants", admin, map[string]any{"agent_id": builderID, "target_kind": "workshop"}, nil); s != http.StatusOK {
		t.Fatalf("workshop grant: %d", s)
	}
	if names := toolNames(t, e, builderTok); !has(names, "extension_draft") || !has(names, "extension_publish") || !has(names, "channel_create") {
		t.Fatalf("meta-tools: %v", names)
	}
	if names := toolNames(t, e, otherTok); has(names, "workshop_list") {
		t.Fatal("another agent got the meta-tools")
	}

	general := e.generalID()
	s, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", builderTok, withHeaders{h: map[string]string{"X-Colonyhall-Channel": general}, next: http.DefaultTransport})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()

	if out, isErr := callIn(t, s, "workshop_list", nil); isErr || !strings.Contains(out, `"slug":"tenor"`) {
		t.Fatalf("list: %s", out)
	}
	if out, isErr := callIn(t, s, "extension_create", map[string]any{"slug": "gif", "title": "GIF-küldés", "bot_handle": "gif-bot"}); isErr {
		t.Fatalf("create: %s", out)
	}
	manifest := map[string]any{"connectors": []string{"tenor"}, "tools": []map[string]any{
		{"name": "search", "kind": "http", "risk": "read", "description": "Find GIFs",
			"input":   map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
			"request": map[string]any{"connector": "tenor", "method": "GET", "path": "/search", "query": map[string]any{"q": "{{input.q}}"}},
			"output":  map[string]any{"pick": map[string]any{"id": "issues[].key"}}},
	}}
	out, isErr := callIn(t, s, "extension_draft", map[string]any{"extension": "gif", "manifest": manifest})
	if isErr || !strings.Contains(out, `"version":1`) || !strings.Contains(out, `"new_tools":["search"]`) {
		t.Fatalf("draft: %s", out)
	}
	if out, isErr := callIn(t, s, "extension_test", map[string]any{"extension": "gif", "version": 1, "tool": "search", "input": map[string]any{"q": "macska"}}); isErr || !strings.Contains(out, "PROJ-1") {
		t.Fatalf("test: %s", out)
	}
	if req := fetch.last(); !strings.Contains(req.URL, "q=macska") || req.Auth["X-Api-Key"] != testExtSecret {
		t.Fatalf("the test's request: %+v", req)
	}

	// Publishing waits for a person's decision on the card.
	type result struct {
		out   string
		isErr bool
	}
	done := make(chan result, 1)
	go func() {
		o, e := callIn(t, s, "extension_publish", map[string]any{"extension": "gif", "version": 1})
		done <- result{o, e}
	}()
	var approvalID string
	waitFor(t, "the approval card", func() bool {
		for _, m := range e.messagesIn(general) {
			if mm := approvalRef.FindStringSubmatch(m.Body); mm != nil && strings.Contains(m.Body, "publish gif v1") {
				approvalID = mm[1]
				return true
			}
		}
		return false
	})
	var ext apispec.Extension
	var list apispec.ExtensionList
	e.do("GET", "/api/v1/workshop/extensions", admin, nil, &list)
	if len(list.Extensions) != 1 || list.Extensions[0].State != "draft" {
		t.Fatalf("live before the decision: %+v", list.Extensions)
	}
	if st := e.do("POST", "/api/v1/approvals/"+approvalID, admin, map[string]any{"decision": "allow"}, nil); st != http.StatusOK {
		t.Fatalf("allow: %d", st)
	}
	select {
	case r := <-done:
		if r.isErr || !strings.Contains(r.out, `"live_version":1`) {
			t.Fatalf("publish: %s", r.out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("publish did not return")
	}
	e.do("GET", "/api/v1/workshop/extensions/"+list.Extensions[0].Id.String(), admin, nil, &ext)
	if ext.State != "live" || ext.Versions[0].CreatedBy.String() != builderID {
		t.Fatalf("live, written by the Builder: %+v", ext)
	}

	// The Builder makes a channel for its routines (as its owner).
	if out, isErr := callIn(t, s, "channel_create", map[string]any{"name": "gifek"}); isErr {
		t.Fatalf("channel: %s", out)
	}

	// The build log in the wiki names the Builder and the approval.
	var page struct {
		Body string `json:"body"`
	}
	if st := e.do("GET", "/api/v1/wiki/"+wiki.Slugify(workshop.BuildLogTitle("gif")), admin, nil, &page); st != http.StatusOK ||
		!strings.Contains(page.Body, "@builder") || !strings.Contains(page.Body, "x_gif__search") || !strings.Contains(page.Body, "@norbi") {
		t.Fatalf("build log: %d %s", st, page.Body)
	}

	// Export and import under another name: a draft.
	var bundle apispec.ExtensionBundle
	if st := e.do("GET", "/api/v1/workshop/extensions/"+ext.Id.String()+"/export", admin, nil, &bundle); st != http.StatusOK ||
		bundle.Files["manifest.json"] == "" || bundle.Files["BUILDLOG.md"] == "" {
		t.Fatalf("export: %d %+v", st, bundle)
	}
	raw, _ := json.Marshal(bundle)
	if strings.Contains(string(raw), testExtSecret) {
		t.Fatal("the export carries the secret")
	}
	var imported apispec.Extension
	if st := e.do("POST", "/api/v1/workshop/extensions/import", admin, map[string]any{"bundle": bundle, "slug": "gif2", "bot_handle": "gif2-bot"}, &imported); st != http.StatusCreated ||
		imported.State != "draft" || len(imported.Versions) != 1 {
		t.Fatalf("import: %d %+v", st, imported)
	}
}

// A Builder whose owner is not an admin gets no meta-tools.
func TestBuilderNeedsAnAdminOwner(t *testing.T) {
	e := extEnv(t, &fakeFetcher{}, workshop.Limits{})
	admin := server.DevAdminToken
	memberID, _ := e.createUser("bela", "Béla")
	builderID, builderTok := e.newAgent("builder", "Építő", nil, nil)
	e.do("PUT", "/api/v1/agents/"+builderID+"/owner", admin, map[string]any{"owner_id": memberID}, nil)
	e.do("POST", "/api/v1/workshop/grants", admin, map[string]any{"agent_id": builderID, "target_kind": "workshop"}, nil)
	if names := toolNames(t, e, builderTok); has(names, "extension_draft") {
		t.Fatal("a member's agent got the meta-tools")
	}
}
