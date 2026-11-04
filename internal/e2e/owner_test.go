package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// Round 38: agents and machines have an owner. A person who is not an admin
// sees only their own agents' details and machines, and only the owner or
// an admin gives a full-access agent a task.

type ownerFixture struct {
	e                          *env
	general                    string
	hostID                     string
	colleagueID, colleagueTok  string
	norbiAgent, norbiAgentTok  string // full access, nobody's
	ownSandbox, ownSandboxTok  string // sandboxed, the colleague's
	ownFull, ownFullTok        string // full access, the colleague's
	otherSandbox, otherSandTok string // sandboxed, nobody's
}

func newOwnerFixture(t *testing.T) ownerFixture {
	e := newEnv(t)
	f := ownerFixture{e: e, general: e.generalID()}
	f.hostID, _ = e.enroll("homelab")
	f.colleagueID, f.colleagueTok = e.createActor("human", "kollega", "member")
	e.do("POST", "/api/v1/channels/"+f.general+"/members", server.DevAdminToken, map[string]any{"actor_id": f.colleagueID}, nil)
	full := map[string]any{"host_id": f.hostID, "client": "claude-code", "model": "haiku", "allow_work": true, "mode": "direct", "root_dir": "/app", "sandbox": false}
	sand := map[string]any{"host_id": f.hostID, "client": "claude-code", "model": "haiku", "allow_work": true, "mode": "compact", "sandbox": true}
	mine := map[string]any{"owner_id": f.colleagueID}
	f.norbiAgent, f.norbiAgentTok = e.newAgent("norbi-full", "Norbi full", full, nil)
	f.otherSandbox, f.otherSandTok = e.newAgent("norbi-sand", "Norbi sandbox", sand, nil)
	f.ownSandbox, f.ownSandboxTok = e.newAgent("kollega-coder", "Coder", sand, mine)
	f.ownFull, f.ownFullTok = e.newAgent("kollega-planner", "Planner", full, mine)
	return f
}

func (f ownerFixture) reportRun(agentTok string) {
	f.e.t.Helper()
	run := map[string]any{"phase": "direct", "client": "claude-code", "model": "haiku", "status": "ok", "items": 1, "channel_id": f.general,
		"input_tokens": 10, "output_tokens": 5, "cache_read_tokens": 0, "cache_write_tokens": 0, "duration_ms": 10}
	if s := f.e.do("POST", "/api/v1/agent/runs", agentTok, run, nil); s != http.StatusCreated {
		f.e.t.Fatalf("report run: %d", s)
	}
}

func TestOwnerSetAtCreationAndChangedByWorkspaceOwnerOnly(t *testing.T) {
	f := newOwnerFixture(t)
	e := f.e
	var a apispec.Actor
	if s := e.do("GET", "/api/v1/agents/"+f.ownSandbox+"/overview", f.colleagueTok, nil, nil); s != http.StatusOK {
		t.Fatalf("own agent overview: %d", s)
	}
	// An agent cannot own anything; a member cannot change owners.
	if s := e.do("PUT", "/api/v1/agents/"+f.norbiAgent+"/owner", server.DevAdminToken, map[string]any{"owner_id": f.ownSandbox}, nil); s != http.StatusBadRequest {
		t.Fatalf("agent as owner: %d", s)
	}
	if s := e.do("PUT", "/api/v1/agents/"+f.norbiAgent+"/owner", f.colleagueTok, map[string]any{"owner_id": f.colleagueID}, nil); s != http.StatusForbidden {
		t.Fatalf("member changes owner: %d", s)
	}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "bad-owner", "api_key": true, "owner_id": f.ownFull}, nil); s != http.StatusBadRequest {
		t.Fatalf("create with an agent as owner: %d", s)
	}
	// The workspace owner gives Norbi's agent to the colleague, then takes it back.
	if s := e.do("PUT", "/api/v1/agents/"+f.norbiAgent+"/owner", server.DevAdminToken, map[string]any{"owner_id": f.colleagueID}, &a); s != http.StatusOK ||
		a.OwnerId == nil || a.OwnerId.String() != f.colleagueID {
		t.Fatalf("set owner: %d %+v", s, a.OwnerId)
	}
	if s := e.do("GET", "/api/v1/agents/"+f.norbiAgent+"/overview", f.colleagueTok, nil, nil); s != http.StatusOK {
		t.Fatalf("overview after the owner changed: %d", s)
	}
	a = apispec.Actor{}
	if s := e.do("PUT", "/api/v1/agents/"+f.norbiAgent+"/owner", server.DevAdminToken, map[string]any{}, &a); s != http.StatusOK || a.OwnerId != nil {
		t.Fatalf("clear owner: %d %+v", s, a.OwnerId)
	}
	if s := e.do("GET", "/api/v1/agents/"+f.norbiAgent+"/overview", f.colleagueTok, nil, nil); s != http.StatusNotFound {
		t.Fatalf("overview after the owner was cleared: %d", s)
	}
	// Audited.
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	var audit apispec.AuditPage
	e.do("GET", "/api/v1/admin/audit?action=actor.owner_changed", server.DevAdminToken, nil, &audit)
	n := 0
	for _, ev := range audit.Entries {
		if ev.Action == "actor.owner_changed" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("audit owner changes: %d %+v", n, audit)
	}
}

func TestMemberSeesOnlyOwnAgentsDetails(t *testing.T) {
	f := newOwnerFixture(t)
	e := f.e
	// Another person's agent: overview, instructions, notes, folders and
	// terminals are not found.
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/agents/" + f.norbiAgent + "/overview"},
		{"GET", "/api/v1/agents/" + f.norbiAgent + "/instructions"},
		{"GET", "/api/v1/agents/" + f.norbiAgent + "/notes"},
		{"GET", "/api/v1/agents/" + f.otherSandbox + "/overview"},
	} {
		if s := e.do(c.method, c.path, f.colleagueTok, nil, nil); s != http.StatusNotFound {
			t.Fatalf("%s %s: %d", c.method, c.path, s)
		}
	}
	for _, op := range []string{"list", "term_list"} {
		body := map[string]any{"op": op, "channel_id": f.general}
		if s := e.do("POST", "/api/v1/agents/"+f.norbiAgent+"/fs", f.colleagueTok, body, nil); s != http.StatusNotFound {
			t.Fatalf("fs %s on another person's agent: %d", op, s)
		}
	}
	// The admin still sees it.
	if s := e.do("GET", "/api/v1/agents/"+f.norbiAgent+"/overview", server.DevAdminToken, nil, nil); s != http.StatusOK {
		t.Fatalf("admin overview: %d", s)
	}

	// Errors: another person's agent's details are not found, the own are shown.
	var foreign, own struct{ ID string }
	e.do("POST", "/api/v1/agent/errors", f.norbiAgentTok, map[string]any{"channel_id": f.general, "summary": "x"}, &foreign)
	e.do("POST", "/api/v1/agent/errors", f.ownSandboxTok, map[string]any{"channel_id": f.general, "summary": "y"}, &own)
	if s := e.do("GET", "/api/v1/errors/"+foreign.ID, f.colleagueTok, nil, nil); s != http.StatusNotFound {
		t.Fatalf("another person's agent's error: %d", s)
	}
	if s := e.do("GET", "/api/v1/errors/"+own.ID, f.colleagueTok, nil, nil); s != http.StatusOK {
		t.Fatalf("own agent's error: %d", s)
	}

	// Runs and usage: only the own agents, whatever the filter says.
	f.reportRun(f.norbiAgentTok)
	f.reportRun(f.ownSandboxTok)
	f.reportRun(f.ownFullTok)
	runsOf := func(token, query string) map[string]int {
		var list apispec.RunList
		if s := e.do("GET", "/api/v1/runs"+query, token, nil, &list); s != http.StatusOK {
			t.Fatalf("runs%s: %d", query, s)
		}
		out := map[string]int{}
		for _, r := range list.Runs {
			out[r.AgentId.String()]++
		}
		return out
	}
	if got := runsOf(f.colleagueTok, ""); len(got) != 2 || got[f.ownSandbox] != 1 || got[f.ownFull] != 1 {
		t.Fatalf("member runs: %v", got)
	}
	if got := runsOf(f.colleagueTok, "?agent_id="+f.norbiAgent); len(got) != 0 {
		t.Fatalf("member runs of another person's agent: %v", got)
	}
	adminID := e.actorIDByHandle("norbi")
	if got := runsOf(f.colleagueTok, "?owner_id="+adminID); len(got) != 0 {
		t.Fatalf("member runs filtered to another person: %v", got)
	}
	if got := runsOf(server.DevAdminToken, ""); len(got) != 3 {
		t.Fatalf("admin runs: %v", got)
	}
	if got := runsOf(server.DevAdminToken, "?owner_id="+f.colleagueID); len(got) != 2 || got[f.norbiAgent] != 0 {
		t.Fatalf("admin runs by person: %v", got)
	}
	usageAgents := func(token, query string) map[string]bool {
		var u apispec.UsageResponse
		if s := e.do("GET", "/api/v1/usage"+query, token, nil, &u); s != http.StatusOK {
			t.Fatalf("usage%s: %d", query, s)
		}
		out := map[string]bool{}
		for _, d := range u.Usage {
			out[d.AgentId.String()] = true
		}
		return out
	}
	if got := usageAgents(f.colleagueTok, ""); len(got) != 2 || got[f.norbiAgent] {
		t.Fatalf("member usage: %v", got)
	}
	if got := usageAgents(server.DevAdminToken, "?owner_id="+f.colleagueID); len(got) != 2 || got[f.norbiAgent] {
		t.Fatalf("admin usage by person: %v", got)
	}

	// Live steps: the list for the channel holds only the own agents' runs.
	step := map[string]any{"kind": "read", "label": "a.md", "state": "done", "started_at": time.Now().UTC().Format(time.RFC3339)}
	for _, tok := range []string{f.norbiAgentTok, f.ownSandboxTok} {
		if s := e.do("POST", "/api/v1/agent/progress", tok, map[string]any{"channel_id": f.general, "run_key": "r1", "steps": []any{step}}, nil); s != http.StatusNoContent {
			t.Fatalf("progress: %d", s)
		}
	}
	var prog struct {
		Runs []struct {
			AgentID string `json:"agent_id"`
		} `json:"runs"`
	}
	if s := e.do("GET", "/api/v1/channels/"+f.general+"/progress", f.colleagueTok, nil, &prog); s != http.StatusOK || len(prog.Runs) != 1 || prog.Runs[0].AgentID != f.ownSandbox {
		t.Fatalf("member progress: %d %+v", s, prog)
	}
}

func TestMemberCannotWakeAnotherPersonsFullAccessAgent(t *testing.T) {
	f := newOwnerFixture(t)
	e := f.e
	// A DM with another person's agent is refused; with an own agent it opens.
	if s := e.do("POST", "/api/v1/dms", f.colleagueTok, map[string]any{"actor_ids": []string{f.norbiAgent}}, nil); s != http.StatusForbidden {
		t.Fatalf("DM with another person's agent: %d", s)
	}
	if s := e.do("POST", "/api/v1/dms", f.colleagueTok, map[string]any{"actor_ids": []string{f.ownFull}}, nil); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("DM with an own agent: %d", s)
	}
	// Mentions in a shared channel: the colleague wakes their own full-access
	// agent and a sandboxed one, never Norbi's full-access agent.
	if _, s := e.send(f.colleagueTok, f.general, map[string]any{"body": "@norbi-full @kollega-planner @norbi-sand nézzétek meg"}); s != http.StatusCreated {
		t.Fatalf("send: %d", s)
	}
	if got := e.inbox(f.norbiAgentTok); len(got) != 0 {
		t.Fatalf("another person's full-access agent woke: %+v", got)
	}
	if got := e.inbox(f.ownFullTok); len(got) != 1 {
		t.Fatalf("own full-access agent: %+v", got)
	}
	if got := e.inbox(f.otherSandTok); len(got) != 1 {
		t.Fatalf("sandboxed agent: %+v", got)
	}
	// A question is a task too: refused for another person's full-access agent.
	if _, isErr := e.mcpCall(f.colleagueTok, "ask", map[string]any{"question": "Mehet?", "to": "@norbi-full", "channel": "#general"}); !isErr {
		t.Fatal("a member asked another person's full-access agent")
	}
	if got := e.inbox(f.norbiAgentTok); len(got) != 0 {
		t.Fatalf("the question woke it: %+v", got)
	}
	if out, isErr := e.mcpCall(f.colleagueTok, "ask", map[string]any{"question": "Mehet?", "to": "@kollega-planner", "channel": "#general"}); isErr {
		t.Fatalf("a member asks their own agent: %s", out)
	}
	// The admin still wakes it.
	if _, s := e.send(server.DevAdminToken, f.general, map[string]any{"body": "@norbi-full te is"}); s != http.StatusCreated {
		t.Fatalf("admin send: %d", s)
	}
	if got := e.inbox(f.norbiAgentTok); len(got) != 1 {
		t.Fatalf("admin mention: %+v", got)
	}
}

func TestMachineOwnerSeesOwnMachineOnly(t *testing.T) {
	f := newOwnerFixture(t)
	e := f.e
	list := func(token string) apispec.HostList {
		var l apispec.HostList
		if s := e.do("GET", "/api/v1/hosts", token, nil, &l); s != http.StatusOK {
			t.Fatalf("hosts: %d", s)
		}
		return l
	}
	if l := list(f.colleagueTok); len(l.Hosts) != 0 {
		t.Fatalf("member sees a machine that is not theirs: %+v", l)
	}
	if s := e.do("PUT", "/api/v1/hosts/"+f.hostID+"/owner", f.colleagueTok, map[string]any{"owner_id": f.colleagueID}, nil); s != http.StatusForbidden {
		t.Fatalf("member sets the machine owner: %d", s)
	}
	var h apispec.Host
	if s := e.do("PUT", "/api/v1/hosts/"+f.hostID+"/owner", server.DevAdminToken, map[string]any{"owner_id": f.colleagueID}, &h); s != http.StatusOK ||
		h.OwnerId == nil || h.OwnerId.String() != f.colleagueID {
		t.Fatalf("set machine owner: %d %+v", s, h.OwnerId)
	}
	l := list(f.colleagueTok)
	if len(l.Hosts) != 1 || len(l.Hosts[0].Agents) != 2 {
		t.Fatalf("own machine with own agents only: %+v", l)
	}
	for _, a := range l.Hosts[0].Agents {
		if a.AgentId.String() != f.ownSandbox && a.AgentId.String() != f.ownFull {
			t.Fatalf("another person's agent on the own machine: %s", a.Handle)
		}
	}
	if l := list(server.DevAdminToken); len(l.Hosts) != 1 || len(l.Hosts[0].Agents) != 4 {
		t.Fatalf("admin view: %+v", l)
	}
}
