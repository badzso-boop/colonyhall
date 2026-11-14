// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"testing"

	"colonyhall/internal/server"
)

type hostAgent struct {
	AgentID          string `json:"agent_id"`
	Handle           string `json:"handle"`
	Client           string `json:"client"`
	Model            string `json:"model"`
	WorkModel        string `json:"work_model"`
	AllowWork        bool   `json:"allow_work"`
	PermissionMode   string `json:"permission_mode"`
	Mode             string `json:"mode"`
	RootDir          string `json:"root_dir"`
	Sandbox          bool   `json:"sandbox"`
	MaxSessionTokens int    `json:"max_session_tokens"`
	DailyTokenLimit  int64  `json:"daily_token_limit"`
	WorktreeRepo     string `json:"worktree_repo"`
	AllowWebRead     bool   `json:"allow_web_read"`
	Grants           []struct {
		ID           string `json:"id"`
		Path         string `json:"path"`
		Mode         string `json:"mode"`
		OutsideRoots *bool  `json:"outside_roots"`
	} `json:"grants"`
}

// enroll pairs a new guard and returns its host id and token.
func (e *env) enroll(name string) (string, string) {
	e.t.Helper()
	var pc struct{ Code string }
	if s := e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc); s != http.StatusCreated {
		e.t.Fatalf("pairing code: %d", s)
	}
	var out struct {
		HostID string `json:"host_id"`
		Token  string `json:"token"`
	}
	if s := e.do("POST", "/api/v1/hosts/enroll", "", map[string]any{"code": pc.Code, "name": name}, &out); s != http.StatusCreated {
		e.t.Fatalf("enroll: %d", s)
	}
	e.reportReady(out.Token)
	return out.HostID, out.Token
}

func TestHostPairingAndAgentTokens(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()

	// Only admins create pairing codes; a code works once, typed loosely.
	if s := e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent creates pairing code: %d", s)
	}
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	loose := ""
	for _, c := range pc.Code {
		if c != '-' {
			loose += string(c | 0x20) // lower case, no dashes
		}
	}
	var enrolled struct {
		HostID string `json:"host_id"`
		Token  string `json:"token"`
	}
	if s := e.do("POST", "/api/v1/hosts/enroll", "", map[string]any{"code": loose, "name": "laptop"}, &enrolled); s != http.StatusCreated {
		t.Fatalf("enroll: %d", s)
	}
	if s := e.do("POST", "/api/v1/hosts/enroll", "", map[string]any{"code": pc.Code, "name": "again"}, nil); s != http.StatusUnauthorized {
		t.Fatalf("code reused: %d", s)
	}
	host, hostToken := enrolled.HostID, enrolled.Token
	if s := e.do("GET", "/api/v1/me", hostToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("a host token is not an actor token: %d", s)
	}

	e.reportReady(hostToken)

	// Not assigned: no token for the agent.
	if s := e.do("POST", "/api/v1/host/agents/"+agent+"/token", hostToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("token for unassigned agent: %d", s)
	}
	var created struct {
		Actor struct{ ID string }
		Host  hostAgent
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "laptop-claude",
		"host": map[string]any{"host_id": host, "client": "claude-code", "model": "haiku", "allow_work": true, "root_dir": "/app"}}, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	assigned := created.Host
	if assigned.Handle != "laptop-claude" || assigned.Model != "haiku" || assigned.Mode != "direct" || assigned.RootDir != "/app" || assigned.Sandbox != false || assigned.PermissionMode != "auto" || assigned.MaxSessionTokens != 150000 || assigned.DailyTokenLimit != 0 {
		t.Fatalf("assigned: %+v", assigned)
	}
	agent = created.Actor.ID

	// Validation: invalid mode, missing root_dir in direct mode or without a sandbox.
	for name, h := range map[string]map[string]any{
		"invalid mode":                {"host_id": host, "client": "claude-code", "mode": "invalid", "root_dir": "/app"},
		"direct without root_dir":     {"host_id": host, "client": "claude-code", "mode": "direct"},
		"compact unsandboxed no root": {"host_id": host, "client": "claude-code", "mode": "compact", "sandbox": false},
	} {
		if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "bad-config", "host": h}, nil); s != http.StatusBadRequest {
			t.Fatalf("%s allowed: %d", name, s)
		}
	}

	var tok struct{ Token string }
	if s := e.do("POST", "/api/v1/host/agents/"+agent+"/token", hostToken, nil, &tok); s != http.StatusCreated {
		t.Fatalf("agent token: %d", s)
	}
	var me struct{ Handle string }
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, &me); s != http.StatusOK || me.Handle != "laptop-claude" {
		t.Fatalf("agent token works: %d %+v", s, me)
	}

	// Deleting (archiving) the agent ends the host's tokens for it.
	if s := e.do("DELETE", "/api/v1/agents/"+agent, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("archive: %d", s)
	}
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("token after archive: %d", s)
	}

	// Revoking the host ends its own token and every agent token it got.
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "laptop-ag",
		"host": map[string]any{"host_id": host, "client": "antigravity", "mode": "compact"}}, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	e.do("POST", "/api/v1/host/agents/"+created.Actor.ID+"/token", hostToken, nil, &tok)
	if s := e.do("PUT", "/api/v1/hosts/"+host+"/state", server.DevAdminToken, map[string]any{"state": "revoked"}, nil); s != http.StatusNoContent {
		t.Fatalf("revoke host: %d", s)
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("revoked host: %d", s)
	}
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("agent token of revoked host: %d", s)
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "too-late",
		"host": map[string]any{"host_id": host, "client": "claude-code", "mode": "compact"}}, nil); s != http.StatusBadRequest {
		t.Fatalf("create on a revoked host: %d", s)
	}
}

func TestHostConfigAndGrants(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("homelab")
	if s := e.do("POST", "/api/v1/host/report", hostToken, map[string]any{"roots": []string{"/work"}, "clients": []any{map[string]any{"name": "claude-code"}},
		"sandbox": "bwrap 0.9.0", "exchange_dir": "/mnt/z/exchange", "exchange_ok": true, "rejected": []any{}}, nil); s != http.StatusNoContent {
		t.Fatalf("report: %d", s)
	}
	var hl struct {
		Hosts []struct {
			Report struct {
				ExchangeDir string `json:"exchange_dir"`
				ExchangeOK  bool   `json:"exchange_ok"`
			} `json:"report"`
		} `json:"hosts"`
	}
	if s := e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hl); s != http.StatusOK {
		t.Fatalf("list hosts: %d", s)
	}
	if len(hl.Hosts) != 1 || hl.Hosts[0].Report.ExchangeDir != "/mnt/z/exchange" || !hl.Hosts[0].Report.ExchangeOK {
		t.Fatalf("unexpected host report in list: %+v", hl)
	}

	compact := map[string]any{"host_id": host, "client": "claude-code", "model": "haiku", "mode": "compact"}
	withGrants := func(grants ...map[string]any) map[string]any {
		return map[string]any{"handle": "homelab-bot", "host": compact, "grants": grants}
	}
	for _, bad := range []string{"work", "/work/../etc", "/work/", "/a//b"} {
		if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, withGrants(map[string]any{"path": bad, "mode": "read"}), nil); s != http.StatusBadRequest {
			t.Fatalf("bad path %q: %d", bad, s)
		}
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, withGrants(map[string]any{"path": "/work", "mode": "admin"}), nil); s != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", s)
	}
	// Outside the roots the machine reported: refused at creation.
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, withGrants(map[string]any{"path": "/etc", "mode": "read"}), nil); s != http.StatusBadRequest {
		t.Fatalf("grant outside the roots: %d", s)
	}
	var created struct{ Actor struct{ ID string } }
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken,
		withGrants(map[string]any{"path": "/work/app", "mode": "write"}, map[string]any{"path": "/work/docs", "mode": "read"}), &created); s != http.StatusCreated {
		t.Fatalf("create with grants: %d", s)
	}
	agent := created.Actor.ID

	// The guard sees its agents and grants; an unchanged config is a 304.
	var cfg struct {
		Version string
		Agents  []hostAgent
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg); s != http.StatusOK {
		t.Fatalf("config: %d", s)
	}
	if len(cfg.Agents) != 1 || len(cfg.Agents[0].Grants) != 2 || cfg.Agents[0].Grants[0].Path != "/work/app" {
		t.Fatalf("config: %+v", cfg)
	}
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/host/config", nil)
	req.Header.Set("Authorization", "Bearer "+hostToken)
	req.Header.Set("If-None-Match", `"`+cfg.Version+`"`)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("unchanged config: %d", res.StatusCode)
	}

	// Admins see the machine with its report and agents.
	var list struct {
		Hosts []struct {
			Name   string
			Report struct{ Sandbox string }
			Agents []hostAgent
		}
	}
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &list)
	if len(list.Hosts) != 1 || list.Hosts[0].Report.Sandbox != "bwrap 0.9.0" || len(list.Hosts[0].Agents) != 1 {
		t.Fatalf("hosts: %+v", list)
	}

	// Deleting the agent takes it out of the guard's config.
	if s := e.do("DELETE", "/api/v1/agents/"+agent, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("archive: %d", s)
	}
	var cfg2 struct {
		Version string
		Agents  []hostAgent
	}
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg2)
	if cfg2.Version == cfg.Version || len(cfg2.Agents) != 0 {
		t.Fatalf("after archive: %+v", cfg2)
	}
	if s := e.do("GET", "/api/v1/hosts", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent lists hosts: %d", s)
	}

	// Every change is in the audit log.
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	var audit struct{ Entries []struct{ Action string } }
	e.do("GET", "/api/v1/admin/audit", server.DevAdminToken, nil, &audit)
	seen := map[string]bool{}
	for _, en := range audit.Entries {
		seen[en.Action] = true
	}
	for _, want := range []string{"host.pairing_created", "host.enrolled", "host.agent_assigned", "host.grant_added", "host.agent_unassigned", "agent.created", "agent.archived", "actor.archived"} {
		if !seen[want] {
			t.Errorf("audit misses %s", want)
		}
	}
}

func TestWorktreeAssignmentAndReport(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("worktree-box")
	try := func(handle string, h map[string]any) (int, hostAgent) {
		var created struct{ Host hostAgent }
		h["host_id"], h["client"] = host, "claude-code"
		s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": handle, "host": h}, &created)
		return s, created.Host
	}

	// 1. worktree_repo with mismatched sandbox modes is refused
	// (direct with sandbox: true, or compact with sandbox: false)
	if s, _ := try("wt-a", map[string]any{"mode": "direct", "sandbox": true, "worktree_repo": "/repo"}); s != http.StatusBadRequest {
		t.Fatalf("worktree_repo in direct mode with sandbox: true should be refused: %d", s)
	}
	if s, _ := try("wt-a", map[string]any{"mode": "compact", "sandbox": false, "worktree_repo": "/repo"}); s != http.StatusBadRequest {
		t.Fatalf("worktree_repo in compact mode with sandbox: false should be refused: %d", s)
	}
	// 2. bad worktree_repo paths
	for _, bad := range []string{"repo", "/repo/../etc", "/repo/", "/a//b"} {
		if s, _ := try("wt-a", map[string]any{"mode": "direct", "worktree_repo": bad}); s != http.StatusBadRequest {
			t.Fatalf("bad worktree_repo path %q allowed: %d", bad, s)
		}
	}
	// 1b. worktree_repo in compact mode with sandbox: true
	s, compactAssigned := try("wt-compact", map[string]any{"mode": "compact", "sandbox": true, "worktree_repo": "/repo"})
	if s != http.StatusCreated || compactAssigned.WorktreeRepo != "/repo" || compactAssigned.Mode != "compact" || !compactAssigned.Sandbox {
		t.Fatalf("worktree_repo in compact mode with sandbox: true: %d %+v", s, compactAssigned)
	}
	// 3. direct mode: root_dir defaults to worktree_repo if not provided
	s, assigned := try("wt-direct", map[string]any{"mode": "direct", "worktree_repo": "/repo"})
	if s != http.StatusCreated || assigned.WorktreeRepo != "/repo" || assigned.RootDir != "/repo" || assigned.Mode != "direct" {
		t.Fatalf("assign with worktree_repo: %d %+v", s, assigned)
	}
	agent := assigned.AgentID

	// 4. Host config returns worktree_repo
	var cfg struct {
		Version string
		Agents  []hostAgent
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg); s != http.StatusOK {
		t.Fatalf("config: %d", s)
	}
	if len(cfg.Agents) != 2 || cfg.Agents[0].WorktreeRepo != "/repo" || cfg.Agents[1].WorktreeRepo != "/repo" {
		t.Fatalf("config worktree_repo mismatch: %+v", cfg)
	}

	// 5. Host report with worktrees
	report := map[string]any{
		"roots":             []string{"/repo"},
		"clients":           []any{map[string]any{"name": "claude-code"}},
		"sandbox":           "bwrap 0.9.0",
		"allow_unsandboxed": true,
		"rejected":          []any{},
		"worktrees": []any{
			map[string]any{
				"agent_id": agent,
				"path":     "/repo-worktrees/fake-agent",
				"branch":   "colony/fake-agent/work",
			},
		},
	}
	if s := e.do("POST", "/api/v1/host/report", hostToken, report, nil); s != http.StatusNoContent {
		t.Fatalf("report with worktrees: %d", s)
	}

	// 6. Admin GET /api/v1/hosts shows reported worktrees
	var list struct {
		Hosts []struct {
			Name   string
			Report struct {
				Worktrees []struct {
					AgentID string `json:"agent_id"`
					Path    string `json:"path"`
					Branch  string `json:"branch"`
				} `json:"worktrees"`
			} `json:"report"`
			Agents []hostAgent
		}
	}
	if s := e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &list); s != http.StatusOK {
		t.Fatalf("get hosts: %d", s)
	}
	if len(list.Hosts) != 1 || len(list.Hosts[0].Report.Worktrees) != 1 {
		t.Fatalf("reported worktrees missing: %+v", list)
	}
	wt := list.Hosts[0].Report.Worktrees[0]
	if wt.AgentID != agent || wt.Path != "/repo-worktrees/fake-agent" || wt.Branch != "colony/fake-agent/work" {
		t.Fatalf("worktree entry mismatch: %+v", wt)
	}
}

func TestHostAssignAllowWebRead(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("laptop-webread")

	// 1. Full-access agent (mode: direct, sandbox: false) with allow_web_read -> 400 Bad Request
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "reader",
		"host": map[string]any{"host_id": host, "client": "claude-code", "mode": "direct", "root_dir": "/app", "allow_web_read": true}}, nil); s != http.StatusBadRequest {
		t.Fatalf("allow_web_read on full-access agent must fail with 400, got: %d", s)
	}

	// 2. Sandboxed agent (mode: compact, sandbox: true) with allow_web_read
	var created struct {
		Actor struct{ ID string }
		Host  hostAgent
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "reader",
		"host": map[string]any{"host_id": host, "client": "claude-code", "mode": "compact", "sandbox": true, "allow_web_read": true}}, &created); s != http.StatusCreated {
		t.Fatalf("sandboxed agent with allow_web_read: %d", s)
	}
	if !created.Host.AllowWebRead {
		t.Fatalf("expected allow_web_read to be true, got false")
	}
	agent := created.Actor.ID

	// 3. Appears in host config
	var cfg struct {
		Agents []hostAgent `json:"agents"`
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg); s != http.StatusOK {
		t.Fatalf("get host config: %d", s)
	}
	if len(cfg.Agents) != 1 || !cfg.Agents[0].AllowWebRead {
		t.Fatalf("expected allow_web_read in host config: %+v", cfg)
	}

	// 4. Appears in agent overview
	var ov struct {
		Assignment *struct {
			AllowWebRead *bool `json:"allow_web_read"`
		} `json:"assignment"`
	}
	if s := e.do("GET", "/api/v1/agents/"+agent+"/overview", server.DevAdminToken, nil, &ov); s != http.StatusOK {
		t.Fatalf("get agent overview: %d", s)
	}
	if ov.Assignment == nil || ov.Assignment.AllowWebRead == nil || !*ov.Assignment.AllowWebRead {
		t.Fatalf("expected allow_web_read=true in agent overview: %+v", ov.Assignment)
	}
}
