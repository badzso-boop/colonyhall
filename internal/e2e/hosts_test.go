package e2e

import (
	"context"
	"net/http"
	"testing"

	"colonyhall/internal/server"
)

type hostAgent struct {
	AgentID        string `json:"agent_id"`
	Handle         string `json:"handle"`
	Client         string `json:"client"`
	Model          string `json:"model"`
	WorkModel      string `json:"work_model"`
	AllowWork      bool   `json:"allow_work"`
	PermissionMode string `json:"permission_mode"`
	Mode           string `json:"mode"`
	RootDir        string `json:"root_dir"`
	WorktreeRepo   string `json:"worktree_repo"`
	Sandbox        bool   `json:"sandbox"`
	Grants         []struct {
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

	// Not assigned yet: no token for the agent.
	if s := e.do("POST", "/api/v1/host/agents/"+agent+"/token", hostToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("token for unassigned agent: %d", s)
	}
	var assigned hostAgent
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{"host_id": host, "client": "claude-code", "model": "haiku", "allow_work": true, "root_dir": "/app"}, &assigned); s != http.StatusOK {
		t.Fatalf("assign: %d", s)
	}
	if assigned.Handle != "fake-agent" || assigned.Model != "haiku" || assigned.Mode != "direct" || assigned.RootDir != "/app" || assigned.Sandbox != false || assigned.PermissionMode != "auto" {
		t.Fatalf("assigned: %+v", assigned)
	}

	// Validation tests: invalid mode and missing root_dir in direct mode.
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{"host_id": host, "client": "claude-code", "mode": "invalid", "root_dir": "/app"}, nil); s != http.StatusBadRequest {
		t.Fatalf("invalid mode allowed: %d", s)
	}
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{"host_id": host, "client": "claude-code", "mode": "direct"}, nil); s != http.StatusBadRequest {
		t.Fatalf("direct mode without root_dir allowed: %d", s)
	}
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{"host_id": host, "client": "claude-code", "mode": "compact", "sandbox": false}, nil); s != http.StatusBadRequest {
		t.Fatalf("compact mode with sandbox=false without root_dir allowed: %d", s)
	}

	var tok struct{ Token string }
	if s := e.do("POST", "/api/v1/host/agents/"+agent+"/token", hostToken, nil, &tok); s != http.StatusCreated {
		t.Fatalf("agent token: %d", s)
	}
	var me struct{ Handle string }
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, &me); s != http.StatusOK || me.Handle != "fake-agent" {
		t.Fatalf("agent token works: %d %+v", s, me)
	}

	// Unassigning ends the host's tokens for that agent.
	if s := e.do("DELETE", "/api/v1/agents/"+agent+"/host", server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("unassign: %d", s)
	}
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("token after unassign: %d", s)
	}

	// Revoking the host ends its own token and every agent token it got.
	e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken, map[string]any{"host_id": host, "client": "antigravity", "mode": "compact"}, nil)
	e.do("POST", "/api/v1/host/agents/"+agent+"/token", hostToken, nil, &tok)
	if s := e.do("PUT", "/api/v1/hosts/"+host+"/state", server.DevAdminToken, map[string]any{"state": "revoked"}, nil); s != http.StatusNoContent {
		t.Fatalf("revoke host: %d", s)
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("revoked host: %d", s)
	}
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("agent token of revoked host: %d", s)
	}
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken, map[string]any{"host_id": host, "client": "claude-code", "mode": "compact"}, nil); s != http.StatusBadRequest {
		t.Fatalf("assign to revoked host: %d", s)
	}
}

func TestHostConfigAndGrants(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	host, hostToken := e.enroll("homelab")

	if s := e.do("POST", "/api/v1/agents/"+agent+"/grants", server.DevAdminToken, map[string]any{"path": "/work", "mode": "write"}, nil); s != http.StatusBadRequest {
		t.Fatalf("grant before assignment: %d", s)
	}
	e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken, map[string]any{"host_id": host, "client": "claude-code", "model": "haiku", "mode": "compact"}, nil)
	for _, bad := range []string{"work", "/work/../etc", "/work/", "/a//b"} {
		if s := e.do("POST", "/api/v1/agents/"+agent+"/grants", server.DevAdminToken, map[string]any{"path": bad, "mode": "read"}, nil); s != http.StatusBadRequest {
			t.Fatalf("bad path %q: %d", bad, s)
		}
	}
	if s := e.do("POST", "/api/v1/agents/"+agent+"/grants", server.DevAdminToken, map[string]any{"path": "/work", "mode": "admin"}, nil); s != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", s)
	}

	var g struct {
		ID           string `json:"id"`
		OutsideRoots bool   `json:"outside_roots"`
	}
	e.do("POST", "/api/v1/agents/"+agent+"/grants", server.DevAdminToken, map[string]any{"path": "/work/app", "mode": "write"}, &g)
	if !g.OutsideRoots {
		t.Fatal("no roots reported yet: the grant must be flagged")
	}
	if s := e.do("POST", "/api/v1/host/report", hostToken, map[string]any{"roots": []string{"/work"}, "clients": []any{map[string]any{"name": "claude-code"}},
		"sandbox": "bwrap 0.9.0", "rejected": []any{}}, nil); s != http.StatusNoContent {
		t.Fatalf("report: %d", s)
	}
	e.do("POST", "/api/v1/agents/"+agent+"/grants", server.DevAdminToken, map[string]any{"path": "/work/docs", "mode": "read"}, &g)
	if g.OutsideRoots {
		t.Fatal("inside the reported roots")
	}

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

	// Revoking a grant changes the config.
	if s := e.do("DELETE", "/api/v1/grants/"+g.ID, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("revoke grant: %d", s)
	}
	var cfg2 struct {
		Version string
		Agents  []hostAgent
	}
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg2)
	if cfg2.Version == cfg.Version || len(cfg2.Agents[0].Grants) != 1 {
		t.Fatalf("after revoke: %+v", cfg2)
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
	for _, want := range []string{"host.pairing_created", "host.enrolled", "host.agent_assigned", "host.grant_added", "host.grant_revoked"} {
		if !seen[want] {
			t.Errorf("audit misses %s", want)
		}
	}
}

func TestWorktreeAssignmentAndReport(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	host, hostToken := e.enroll("worktree-box")

	// 1. Invalid assignment: worktree_repo is only allowed in direct mode
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{"host_id": host, "client": "claude-code", "mode": "compact", "worktree_repo": "/repo"}, nil); s != http.StatusBadRequest {
		t.Fatalf("worktree_repo in compact mode allowed: %d", s)
	}

	// 2. Invalid assignment: bad path for worktree_repo
	for _, bad := range []string{"repo", "/repo/../etc", "/repo/", "/a//b"} {
		if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
			map[string]any{"host_id": host, "client": "claude-code", "mode": "direct", "worktree_repo": bad}, nil); s != http.StatusBadRequest {
			t.Fatalf("bad worktree_repo path %q allowed: %d", bad, s)
		}
	}

	// 3. Valid assignment in direct mode: root_dir defaults to worktree_repo if not provided
	var assigned hostAgent
	if s := e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{"host_id": host, "client": "claude-code", "mode": "direct", "worktree_repo": "/repo"}, &assigned); s != http.StatusOK {
		t.Fatalf("assign with worktree_repo: %d", s)
	}
	if assigned.WorktreeRepo != "/repo" || assigned.RootDir != "/repo" || assigned.Mode != "direct" {
		t.Fatalf("unexpected assignment: %+v", assigned)
	}

	// 4. Host config returns worktree_repo
	var cfg struct {
		Version string
		Agents  []hostAgent
	}
	if s := e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg); s != http.StatusOK {
		t.Fatalf("config: %d", s)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].WorktreeRepo != "/repo" {
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
