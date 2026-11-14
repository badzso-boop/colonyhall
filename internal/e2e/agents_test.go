// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// Round 33: agents are configured once (POST /agents) and never edited;
// deleting archives them.

func TestAgentConfigEndpointsClosed(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	host, _ := e.enroll("box")
	var p struct{ Type, Detail string }
	for _, c := range []struct{ method, path string }{
		{"PUT", "/api/v1/agents/" + agent + "/host"},
		{"DELETE", "/api/v1/agents/" + agent + "/host"},
		{"POST", "/api/v1/agents/" + agent + "/grants"},
		{"DELETE", "/api/v1/grants/" + agent},
	} {
		var body any
		if c.method == "PUT" {
			body = map[string]any{"host_id": host, "client": "claude-code", "root_dir": "/app"}
		}
		if c.method == "POST" {
			body = map[string]any{"path": "/app", "mode": "read"}
		}
		if s := e.do(c.method, c.path, server.DevAdminToken, body, &p); s != http.StatusConflict || p.Type != "agent_config_immutable" ||
			!strings.Contains(p.Detail, "POST /api/v1/agents") {
			t.Fatalf("%s %s: %d %+v", c.method, c.path, s, p)
		}
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, map[string]any{"kind": "agent", "handle": "old-way"}, &p); s != http.StatusConflict || p.Type != "agent_config_immutable" {
		t.Fatalf("createActor kind agent: %d %+v", s, p)
	}
	// Humans are still created there.
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken, map[string]any{"kind": "human", "handle": "anna"}, nil); s != http.StatusCreated {
		t.Fatalf("human: %d", s)
	}
}

func TestCreateAgentIsAtomicAndChecked(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	host, hostToken := e.enroll("box")
	e.do("POST", "/api/v1/host/report", hostToken, map[string]any{"roots": []string{"/work"},
		"clients": []any{map[string]any{"name": "claude-code", "version": "2"}}}, nil)
	general := e.generalID()
	hostBody := map[string]any{"host_id": host, "client": "claude-code", "model": "sonnet", "mode": "compact"}

	// A grant outside the reported roots: nothing at all is written.
	bad := map[string]any{"handle": "atomic", "host": hostBody, "channel_ids": []string{general},
		"grants": []map[string]any{{"path": "/work/app", "mode": "write"}, {"path": "/etc", "mode": "read"}}, "instructions": "Légy tömör."}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, bad, nil); s != http.StatusBadRequest {
		t.Fatalf("grant outside the roots: %d", s)
	}
	var all struct{ Actors []struct{ Handle string } }
	e.do("GET", "/api/v1/actors?archived=true", server.DevAdminToken, nil, &all)
	for _, a := range all.Actors {
		if a.Handle == "atomic" {
			t.Fatal("a refused agent was written")
		}
	}
	var cfg struct{ Agents []hostAgent }
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	if len(cfg.Agents) != 0 {
		t.Fatalf("a refused agent reached the guard: %+v", cfg)
	}

	// Other refusals before any write.
	for name, body := range map[string]map[string]any{
		"client the host did not report": {"handle": "x1", "host": map[string]any{"host_id": host, "client": "antigravity", "mode": "compact"}},
		"api key with a host":            {"handle": "x2", "api_key": true, "host": hostBody},
		"neither host nor api key":       {"handle": "x3"},
		"secret in instructions":         {"handle": "x4", "host": hostBody, "instructions": "Use GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123 for pushes."},
		"private key in instructions":    {"handle": "x5", "host": hostBody, "instructions": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc"},
		"too long instructions":          {"handle": "x6", "host": hostBody, "instructions": strings.Repeat("a", 8001)},
		"unknown channel":                {"handle": "x7", "host": hostBody, "channel_ids": []string{host}},
		"bad handle":                     {"handle": "X Y", "host": hostBody},
		"colour not in the palette":      {"handle": "x8", "host": hostBody, "color": "accent"},
	} {
		if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, body, nil); s != http.StatusBadRequest {
			t.Fatalf("%s: %d", name, s)
		}
	}

	// Only human admins create agents.
	_, memberToken := e.createActor("human", "anna", "member")
	ok := map[string]any{"handle": "atomic", "host": hostBody, "channel_ids": []string{general},
		"grants": []map[string]any{{"path": "/work/app", "mode": "write"}}, "instructions": "Légy tömör, @{handle}.", "color": "teal"}
	for _, tok := range []string{memberToken, server.DevAgentToken} {
		if s := e.do("POST", "/api/v1/agents", tok, ok, nil); s != http.StatusForbidden {
			t.Fatalf("non-admin creates an agent: %d", s)
		}
	}

	// The whole agent in one call.
	var created struct {
		Actor struct {
			ID, Handle, Color string
		}
		Host  hostAgent
		Token *string
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, ok, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	if created.Actor.Handle != "atomic" || created.Actor.Color != "teal" || created.Token != nil ||
		created.Host.Mode != "compact" || !created.Host.Sandbox || len(created.Host.Grants) != 1 || created.Host.Grants[0].Path != "/work/app" {
		t.Fatalf("created: %+v", created)
	}
	members, err := e.app.Conversation.MemberIDs(ctx, mustID(general))
	if err != nil {
		t.Fatal(err)
	}
	if !containsID(members, created.Actor.ID) {
		t.Fatal("not in #general")
	}
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	if len(cfg.Agents) != 1 || cfg.Agents[0].Handle != "atomic" {
		t.Fatalf("config: %+v", cfg)
	}

	// Own instructions: humans read them with the effective text; agents never.
	var ins struct {
		Text, Effective string
		Version         *int
	}
	if s := e.do("GET", "/api/v1/agents/"+created.Actor.ID+"/instructions", server.DevAdminToken, nil, &ins); s != http.StatusOK ||
		ins.Text != "Légy tömör, @{handle}." || ins.Version == nil || *ins.Version != 1 ||
		!strings.Contains(ins.Effective, "## Instructions for @atomic\n\nLégy tömör, @atomic.") {
		t.Fatalf("instructions: %d %+v", s, ins)
	}
	if s := e.do("GET", "/api/v1/agents/"+created.Actor.ID+"/instructions", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("an agent reads another agent's instructions: %d", s)
	}
	// The agent itself receives them after the shared rules.
	var tok struct{ Token string }
	e.do("POST", "/api/v1/host/agents/"+created.Actor.ID+"/token", hostToken, nil, &tok)
	var own struct{ Text string }
	if s := e.do("GET", "/api/v1/agent/instructions", tok.Token, nil, &own); s != http.StatusOK || !strings.HasSuffix(own.Text, "## Instructions for @atomic\n\nLégy tömör, @atomic.") {
		t.Fatalf("delivery: %d %q", s, own.Text)
	}

	// The handle is taken while the agent is active.
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, ok, nil); s != http.StatusConflict {
		t.Fatalf("handle of an active agent: %d", s)
	}
}

func TestAgentLimits(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("box")
	id, _ := e.newAgent("limited", "Limited", map[string]any{"host_id": host, "client": "claude-code", "mode": "compact"}, nil)
	_, memberToken := e.createActor("human", "anna", "member")
	for _, tok := range []string{memberToken, server.DevAgentToken} {
		if s := e.do("PATCH", "/api/v1/agents/"+id+"/limits", tok, map[string]any{"daily_token_limit": 1}, nil); s != http.StatusForbidden {
			t.Fatalf("non-admin changes limits: %d", s)
		}
	}
	if s := e.do("PATCH", "/api/v1/agents/"+id+"/limits", server.DevAdminToken, map[string]any{"daily_token_limit": -1}, nil); s != http.StatusBadRequest {
		t.Fatalf("negative limit: %d", s)
	}
	var l struct {
		Daily   int64 `json:"daily_token_limit"`
		Session int   `json:"max_session_tokens"`
	}
	if s := e.do("PATCH", "/api/v1/agents/"+id+"/limits", server.DevAdminToken, map[string]any{"daily_token_limit": 400000, "max_session_tokens": 300000}, &l); s != http.StatusOK ||
		l.Daily != 400000 || l.Session != 300000 {
		t.Fatalf("limits: %d %+v", s, l)
	}
	var cfg struct{ Agents []hostAgent }
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	if len(cfg.Agents) != 1 || cfg.Agents[0].DailyTokenLimit != 400000 || cfg.Agents[0].MaxSessionTokens != 300000 || cfg.Agents[0].Mode != "compact" {
		t.Fatalf("config: %+v", cfg)
	}
}

func TestArchiveAgentAndReuseHandle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()
	host, hostToken := e.enroll("box")
	old, oldToken := e.newAgent("backend", "Backend", map[string]any{"host_id": host, "client": "claude-code", "mode": "compact"}, nil)
	var hostTok struct{ Token string }
	e.do("POST", "/api/v1/host/agents/"+old+"/token", hostToken, nil, &hostTok)

	_, memberToken := e.createActor("human", "anna", "member")
	for _, tok := range []string{memberToken, server.DevAgentToken} {
		if s := e.do("DELETE", "/api/v1/agents/"+old, tok, nil, nil); s != http.StatusForbidden {
			t.Fatalf("non-admin deletes an agent: %d", s)
		}
	}
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me)
	if s := e.do("DELETE", "/api/v1/agents/"+me.ID, server.DevAdminToken, nil, nil); s != http.StatusBadRequest {
		t.Fatalf("delete a human with the agent endpoint: %d", s)
	}
	e.send(server.DevAdminToken, general, map[string]any{"body": "@backend before"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var ov struct {
		Inbox *struct {
			Count    int     `json:"count"`
			OldestAt *string `json:"oldest_at"`
		} `json:"inbox"`
	}
	if s := e.do("GET", "/api/v1/agents/"+old+"/overview", server.DevAdminToken, nil, &ov); s != http.StatusOK || ov.Inbox == nil || ov.Inbox.Count != 1 || ov.Inbox.OldestAt == nil {
		t.Fatalf("overview inbox: %d %+v", s, ov.Inbox)
	}
	var memberOv struct{ Inbox *struct{ Count int } }
	e.do("GET", "/api/v1/agents/"+old+"/overview", memberToken, nil, &memberOv)
	if memberOv.Inbox != nil {
		t.Fatal("a member sees the inbox")
	}
	if s := e.do("DELETE", "/api/v1/agents/"+old, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("archive: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/agents/"+old, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("archive twice: %d", s)
	}
	// Tokens end, the guard drops it, memberships go, its pending inbox is acked.
	for _, tok := range []string{oldToken, hostTok.Token} {
		if s := e.do("GET", "/api/v1/me", tok, nil, nil); s != http.StatusUnauthorized {
			t.Fatalf("token after archive: %d", s)
		}
	}
	if s := e.do("POST", "/api/v1/host/agents/"+old+"/token", hostToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("new host token after archive: %d", s)
	}
	var cfg struct{ Agents []hostAgent }
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	if len(cfg.Agents) != 0 {
		t.Fatalf("the guard still has it: %+v", cfg)
	}
	members, _ := e.app.Conversation.MemberIDs(ctx, mustID(general))
	if containsID(members, old) {
		t.Fatal("still a member of #general")
	}
	pending, _, err := e.app.Delivery.Store.Pending(ctx, mustID(old), 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending inbox after archive: %d %v", len(pending), err)
	}

	// Archived: out of /actors, in ?archived=true with archived_at.
	var list struct {
		Actors []struct {
			ID, Handle string
			ArchivedAt *string `json:"archived_at"`
		}
	}
	e.do("GET", "/api/v1/actors", server.DevAdminToken, nil, &list)
	for _, a := range list.Actors {
		if a.ID == old {
			t.Fatal("an archived agent is listed")
		}
	}
	e.do("GET", "/api/v1/actors?archived=true", server.DevAdminToken, nil, &list)
	found := false
	for _, a := range list.Actors {
		if a.ID == old {
			found = a.ArchivedAt != nil
		}
	}
	if !found {
		t.Fatal("archived agent missing from ?archived=true")
	}

	// The handle is free: a new @backend gets the mentions; the old one keeps its messages.
	fresh, freshToken := e.newAgent("backend", "Backend 2", nil, nil)
	if fresh == old {
		t.Fatal("same id")
	}
	e.send(server.DevAdminToken, general, map[string]any{"body": "@backend after"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	items := e.inbox(freshToken)
	if len(items) == 0 || !strings.Contains(items[len(items)-1].Body, "after") {
		t.Fatalf("the mention did not reach the new agent: %+v", items)
	}
	pending, _, _ = e.app.Delivery.Store.Pending(ctx, mustID(old), 10)
	if len(pending) != 0 {
		t.Fatalf("the archived agent was woken: %d", len(pending))
	}
	// Limits of an archived agent cannot change.
	if s := e.do("PATCH", "/api/v1/agents/"+old+"/limits", server.DevAdminToken, map[string]any{"daily_token_limit": 1}, nil); s != http.StatusConflict {
		t.Fatalf("limits of an archived agent: %d", s)
	}
}

func containsID(list []ids.ID, id string) bool {
	for _, x := range list {
		if x.String() == id {
			return true
		}
	}
	return false
}

// Round 41: a machine without a sandbox (macOS) takes full-access agents only.
func TestSandboxedAgentRefusedOnAMachineWithoutSandbox(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("mac")
	report := map[string]any{"guard_version": "v0.3.0", "os": "darwin/arm64", "roots": []string{"/Users/a"},
		"clients": []map[string]any{{"name": "claude-code", "version": "2"}}}
	if s := e.do("POST", "/api/v1/host/report", hostToken, report, nil); s != http.StatusNoContent && s != http.StatusOK {
		t.Fatalf("report: %d", s)
	}
	var p struct{ Detail string }
	sandboxed := map[string]any{"handle": "mac-sand", "host": map[string]any{"host_id": host, "client": "claude-code", "mode": "compact", "sandbox": true}}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, sandboxed, &p); s != http.StatusBadRequest || !strings.Contains(p.Detail, "no sandbox") {
		t.Fatalf("sandboxed on macOS: %d %q", s, p.Detail)
	}
	full := map[string]any{"handle": "mac-full", "host": map[string]any{"host_id": host, "client": "claude-code", "mode": "direct", "sandbox": false, "root_dir": "/Users/a"}}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, full, nil); s != http.StatusCreated {
		t.Fatalf("full access on macOS: %d", s)
	}
}

// Round 41: the guard's doctor checks reach the Machines page.
func TestHostReportCarriesTheDoctor(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("box")
	report := map[string]any{"guard_version": "v0.3.0", "os": "linux/amd64", "sandbox": "bwrap 0.9.0", "roots": []string{"/w"}, "clients": []any{},
		"doctor":            []map[string]any{{"id": "claude", "status": "warn", "detail": "not logged in"}, {"id": "tmux", "status": "ok"}},
		"doctor_checked_at": "2026-11-05T10:00:00Z"}
	if s := e.do("POST", "/api/v1/host/report", hostToken, report, nil); s != http.StatusNoContent && s != http.StatusOK {
		t.Fatalf("report: %d", s)
	}
	var list apispec.HostList
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &list)
	for _, h := range list.Hosts {
		if h.Id.String() == host {
			if h.Report.Doctor == nil || len(*h.Report.Doctor) != 2 || (*h.Report.Doctor)[0].Status != "warn" {
				t.Fatalf("doctor: %+v", h.Report.Doctor)
			}
			return
		}
	}
	t.Fatal("host missing")
}

// Round 39: CodeGraph's version and indexed repositories reach the Machines
// page; an unknown status from a guard is stored as failed, long fields are
// clipped to the contract's limits.
func TestHostReportCarriesCodeGraph(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("box")
	report := map[string]any{"guard_version": "v0.3.0", "os": "linux/amd64", "roots": []string{"/w"}, "clients": []any{},
		"codegraph": map[string]any{"version": "1.6.2", "repos": []map[string]any{
			{"path": "/w/app", "status": "indexed", "files": 807, "indexed_at": "2026-11-08T10:00:00Z"},
			{"path": "/w/huge", "status": "skipped", "files": 90000, "detail": strings.Repeat("x", 500)},
			{"path": "/w/odd", "status": "exploded"},
		}}}
	if s := e.do("POST", "/api/v1/host/report", hostToken, report, nil); s != http.StatusNoContent && s != http.StatusOK {
		t.Fatalf("report: %d", s)
	}
	var list apispec.HostList
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &list)
	for _, h := range list.Hosts {
		if h.Id.String() != host {
			continue
		}
		cg := h.Report.Codegraph
		if cg == nil || cg.Version != "1.6.2" || len(cg.Repos) != 3 {
			t.Fatalf("codegraph: %+v", cg)
		}
		if r := cg.Repos[0]; r.Path != "/w/app" || r.Status != apispec.CodegraphRepoStatusIndexed || r.Files == nil || *r.Files != 807 || r.IndexedAt == nil {
			t.Fatalf("repo 0: %+v", r)
		}
		if d := cg.Repos[1].Detail; d == nil || len(*d) != 200 {
			t.Fatalf("detail not clipped: %v", d)
		}
		if cg.Repos[2].Status != apispec.CodegraphRepoStatusFailed {
			t.Fatalf("unknown status kept: %q", cg.Repos[2].Status)
		}
		return
	}
	t.Fatal("host missing")
}
