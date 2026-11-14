// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/server"
)

// Round 38: a docs-only planner is a full-access Claude Code agent that
// writes only Markdown and hands code to a sandboxed partner, both fixed
// at creation.
func TestDocsOnlyAgentCreation(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("box")
	coderID, _ := e.newAgent("coder", "Coder", map[string]any{"host_id": host, "client": "claude-code", "mode": "compact", "sandbox": true}, nil)
	fullID, _ := e.newAgent("helper", "Helper", map[string]any{"host_id": host, "client": "claude-code", "mode": "direct", "root_dir": "/work"}, nil)
	humanID, _ := e.createActor("human", "anna", "member")

	planner := func(extra map[string]any) map[string]any {
		h := map[string]any{"host_id": host, "client": "claude-code", "mode": "direct", "root_dir": "/work/docs",
			"write_scope": "docs", "partner_id": coderID}
		for k, v := range extra {
			h[k] = v
		}
		return h
	}
	for name, h := range map[string]map[string]any{
		"sandboxed":                  planner(map[string]any{"sandbox": true}),
		"antigravity":                planner(map[string]any{"client": "antigravity"}),
		"compact":                    planner(map[string]any{"mode": "compact", "sandbox": false}),
		"chat approvals":             planner(map[string]any{"approvals": "chat"}),
		"preview":                    planner(map[string]any{"allow_preview": true}),
		"no partner":                 planner(map[string]any{"partner_id": nil}),
		"full-access partner":        planner(map[string]any{"partner_id": fullID}),
		"human partner":              planner(map[string]any{"partner_id": humanID}),
		"unknown partner":            planner(map[string]any{"partner_id": host}),
		"partner without docs scope": planner(map[string]any{"write_scope": "all"}),
		"unknown scope":              planner(map[string]any{"write_scope": "code"}),
	} {
		if h["partner_id"] == nil {
			delete(h, "partner_id")
		}
		var p struct{ Detail string }
		if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "planner", "host": h}, &p); s != http.StatusBadRequest {
			t.Fatalf("%s: %d %+v", name, s, p)
		}
	}

	var created struct {
		Actor struct{ ID string }
		Host  struct {
			WriteScope    string `json:"write_scope"`
			PartnerID     string `json:"partner_id"`
			PartnerHandle string `json:"partner_handle"`
		}
	}
	body := map[string]any{"handle": "planner", "host": planner(nil), "instructions": "Write the specs in docs/."}
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, body, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	if created.Host.WriteScope != "docs" || created.Host.PartnerID != coderID || created.Host.PartnerHandle != "coder" {
		t.Fatalf("created: %+v", created)
	}

	// The guard learns the scope and the partner; other agents are "all".
	var cfg struct {
		Agents []struct {
			Handle        string `json:"handle"`
			WriteScope    string `json:"write_scope"`
			PartnerHandle string `json:"partner_handle"`
		}
	}
	e.do("GET", "/api/v1/host/config", hostToken, nil, &cfg)
	scopes := map[string]string{}
	for _, a := range cfg.Agents {
		scopes[a.Handle] = a.WriteScope + "/" + a.PartnerHandle
	}
	if scopes["planner"] != "docs/coder" || scopes["coder"] != "all/" || scopes["helper"] != "all/" {
		t.Fatalf("host config: %v", scopes)
	}

	// The fixed block follows the own instructions, for humans and for the
	// agent itself.
	var ins struct{ Text, Effective string }
	if s := e.do("GET", "/api/v1/agents/"+created.Actor.ID+"/instructions", server.DevAdminToken, nil, &ins); s != http.StatusOK {
		t.Fatalf("instructions: %d", s)
	}
	if ins.Text != "Write the specs in docs/." || !strings.Contains(ins.Effective, "Write the specs in docs/.\n\n## Docs-only planner") ||
		!strings.Contains(ins.Effective, "sandboxed partner @coder") || !strings.Contains(ins.Effective, "Never ask a human to run code") {
		t.Fatalf("instructions: %+v", ins)
	}
	var tok struct{ Token string }
	e.do("POST", "/api/v1/host/agents/"+created.Actor.ID+"/token", hostToken, nil, &tok)
	var own struct{ Text string }
	if s := e.do("GET", "/api/v1/agent/instructions", tok.Token, nil, &own); s != http.StatusOK ||
		!strings.Contains(own.Text, "## Docs-only planner") || !strings.Contains(own.Text, "@coder") {
		t.Fatalf("delivery: %d %q", s, own.Text)
	}

	// Without own instructions the block is still there.
	body2 := map[string]any{"handle": "planner2", "host": planner(nil)}
	var created2 struct{ Actor struct{ ID string } }
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, body2, &created2); s != http.StatusCreated {
		t.Fatalf("create planner2: %d", s)
	}
	var ins2 struct{ Text, Effective string }
	e.do("GET", "/api/v1/agents/"+created2.Actor.ID+"/instructions", server.DevAdminToken, nil, &ins2)
	if ins2.Text != "" || !strings.Contains(ins2.Effective, "## Instructions for @planner2\n\n## Docs-only planner") {
		t.Fatalf("planner2 instructions: %+v", ins2)
	}

	// The admin's agent overview shows the mode.
	var ov struct {
		Assignment struct {
			WriteScope    string `json:"write_scope"`
			PartnerHandle string `json:"partner_handle"`
		} `json:"assignment"`
	}
	if s := e.do("GET", "/api/v1/agents/"+created.Actor.ID+"/overview", server.DevAdminToken, nil, &ov); s != http.StatusOK ||
		ov.Assignment.WriteScope != "docs" || ov.Assignment.PartnerHandle != "coder" {
		t.Fatalf("overview: %d %+v", s, ov)
	}
}
