package e2e

import (
	"context"
	"encoding/json"
	"net/http"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/hosts"
	"colonyhall/internal/platform/ids"
	"colonyhall/internal/server"
)

// Agents are configured once, with POST /agents (round 33). These helpers
// create them that way; configure() is the one exception, for the seeded
// @fake-agent whose fixed token many tests use.

// reportReady makes an enrolled host able to take agents: it reports both
// clients and "/" as its root, as a guard would.
func (e *env) reportReady(hostToken string) {
	e.t.Helper()
	report := map[string]any{"guard_version": "test", "roots": []string{"/"},
		"clients": []map[string]any{{"name": "claude-code", "version": "1"}, {"name": "antigravity", "version": "1"}}}
	if s := e.do("POST", "/api/v1/host/report", hostToken, report, nil); s != http.StatusNoContent && s != http.StatusOK {
		e.t.Fatalf("host report: %d", s)
	}
}

// newAgent creates an agent with POST /agents and gives it an API token
// (a guard-run agent has none of its own; tests act as it with this one).
// host may be nil for an API-key agent; extra adds fields (grants, …).
func (e *env) newAgent(handle, name string, host, extra map[string]any) (string, string) {
	e.t.Helper()
	body := map[string]any{"handle": handle, "display_name": name, "channel_ids": []string{e.generalID()}}
	if host == nil {
		body["api_key"] = true
	} else {
		body["host"] = host
	}
	for k, v := range extra {
		body[k] = v
	}
	var created struct {
		Actor apispec.Actor `json:"actor"`
		Token string        `json:"token"`
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, body, &created); s != http.StatusCreated {
		e.t.Fatalf("create agent @%s: %d", handle, s)
	}
	id := created.Actor.Id.String()
	if created.Token != "" {
		return id, created.Token
	}
	var issued struct{ Secret string }
	if s := e.do("POST", "/api/v1/actors/"+id+"/tokens", server.DevAdminToken, map[string]any{"label": "test"}, &issued); s != http.StatusCreated {
		e.t.Fatalf("token for @%s: %d", handle, s)
	}
	return id, issued.Secret
}

// configure gives an existing agent (the seeded @fake-agent) a machine
// configuration in-process, as POST /agents would have. Only tests do this:
// the API has no way to configure an agent after creation.
func (e *env) configure(agentID string, assign map[string]any, grants ...map[string]any) hostAgent {
	e.t.Helper()
	ctx := context.Background()
	raw, _ := json.Marshal(assign)
	var req apispec.AssignRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		e.t.Fatalf("configure: %v", err)
	}
	in := hosts.AssignInput{
		HostID: req.HostId, Client: string(req.Client), Model: deref(req.Model), WorkModel: deref(req.WorkModel),
		AllowWork: req.AllowWork != nil && *req.AllowWork, PermissionMode: deref(req.PermissionMode), RootDir: deref(req.RootDir),
		WorktreeRepo: deref(req.WorktreeRepo), Sandbox: req.Sandbox, MaxSessionTokens: req.MaxSessionTokens, DailyTokenLimit: req.DailyTokenLimit,
		AllowPreview: req.AllowPreview != nil && *req.AllowPreview, AllowWebRead: req.AllowWebRead != nil && *req.AllowWebRead,
	}
	if req.Mode != nil {
		in.Mode = string(*req.Mode)
	}
	if req.Approvals != nil {
		in.Approvals = string(*req.Approvals)
	}
	var gs []hosts.GrantInput
	for _, g := range grants {
		gs = append(gs, hosts.GrantInput{Path: g["path"].(string), Mode: g["mode"].(string)})
	}
	admin, err := e.app.Identity.ActorByHandle(ctx, e.workspaceID(), "norbi")
	if err != nil {
		e.t.Fatalf("configure: %v", err)
	}
	agent, err := e.app.Identity.ActorByID(ctx, mustID(agentID))
	if err != nil {
		e.t.Fatalf("configure: %v", err)
	}
	// A host made without e.enroll has not reported yet: report as a guard would.
	if h, err := e.app.Hosts.Store.HostByID(ctx, in.HostID); err == nil && len(h.Report.Clients) == 0 {
		rep := hosts.Report{GuardVersion: "test", Roots: []string{"/"}, Clients: []hosts.Client{{Name: "claude-code", Version: "1"}, {Name: "antigravity", Version: "1"}}}
		if err := e.app.Hosts.SaveReport(ctx, h, rep); err != nil {
			e.t.Fatalf("configure: report: %v", err)
		}
	}
	a, gs, err := e.app.Hosts.ValidateNewAgent(ctx, admin, in, gs)
	if err != nil {
		e.t.Fatalf("configure @%s: %v", agent.Handle, err)
	}
	cfg, err := e.app.Hosts.CreateAgentConfig(ctx, admin, agent, a, gs)
	if err != nil {
		e.t.Fatalf("configure @%s: %v", agent.Handle, err)
	}
	var out hostAgent
	raw, _ = json.Marshal(hostAgentJSON(cfg))
	_ = json.Unmarshal(raw, &out)
	return out
}

// grantDirectly stores a folder grant bypassing the server's checks, as
// one made before the machine's roots changed.
func (e *env) grantDirectly(agentID, hostID, path, mode string) {
	e.t.Helper()
	g := hosts.Grant{ID: ids.V7{}.New(), WorkspaceID: e.workspaceID(), AgentID: mustID(agentID), HostID: mustID(hostID), Path: path, Mode: mode,
		CreatedBy: mustID(agentID), CreatedAt: e.clock.Now()}
	if err := e.app.Hosts.Store.AddGrant(context.Background(), g); err != nil {
		e.t.Fatalf("grant: %v", err)
	}
}

func mustID(s string) ids.ID {
	id, err := ids.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (e *env) workspaceID() ids.ID {
	e.t.Helper()
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me)
	a, err := e.app.Identity.ActorByID(context.Background(), mustID(me.ID))
	if err != nil {
		e.t.Fatalf("workspace: %v", err)
	}
	return a.WorkspaceID
}

func hostAgentJSON(c hosts.AgentConfig) map[string]any {
	grants := []map[string]any{}
	for _, g := range c.Grants {
		grants = append(grants, map[string]any{"id": g.ID.String(), "path": g.Path, "mode": g.Mode})
	}
	return map[string]any{"agent_id": c.AgentID.String(), "handle": c.Handle, "client": c.Client, "model": c.Model,
		"work_model": c.WorkModel, "allow_work": c.AllowWork, "permission_mode": c.PermissionMode, "mode": c.Mode,
		"root_dir": c.RootDir, "sandbox": c.Sandbox, "max_session_tokens": c.MaxSessionTokens, "daily_token_limit": c.DailyTokenLimit,
		"worktree_repo": c.WorktreeRepo, "allow_web_read": c.AllowWebRead, "grants": grants}
}
