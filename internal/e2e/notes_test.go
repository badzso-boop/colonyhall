// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

func TestNotesRESTAndMCP(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	general := e.generalID()

	// Post a root message in #general to establish a thread
	msg, s := e.send(server.DevAdminToken, general, map[string]any{
		"body": "Root thread message",
	})
	if s != http.StatusCreated {
		t.Fatalf("create root message: %d", s)
	}
	threadRootID := msg.ID

	// Create two agents
	agentAID, agentAToken := e.createAgent("agent-alpha", "Agent Alpha")
	agentBID, agentBToken := e.createAgent("agent-beta", "Agent Beta")

	// 1. Agent Alpha connects via MCP and sets a note using thread parameter
	sessionA, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agentAToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sessionA.Close() }()

	res, err := sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "note_set",
		Arguments: map[string]any{
			"thread": threadRootID,
			"text":   "Alpha initial task: investigate performance bug.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		var errMsg string
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				errMsg = tc.Text
			}
		}
		t.Fatalf("note_set failed: %s", errMsg)
	}

	// note_get by Agent Alpha returns the note
	res, err = sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "note_get",
		Arguments: map[string]any{
			"thread": threadRootID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("note_get failed: %+v", res)
	}
	if textContent, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.Contains(textContent.Text, "Alpha initial task") {
		t.Fatalf("unexpected note_get content: %+v", res.Content)
	}

	// 2. Agent Beta connects via MCP to the same thread - must NOT see Alpha's note
	sessionB, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agentBToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sessionB.Close() }()

	res, err = sessionB.CallTool(ctx, &mcp.CallToolParams{
		Name: "note_get",
		Arguments: map[string]any{
			"thread": threadRootID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("note_get for beta failed: %+v", res)
	}
	if textContent, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.EqualFold(strings.TrimSpace(textContent.Text), "no note for this thread yet.") {
		t.Fatalf("expected empty note for Beta, got: %+v", res.Content)
	}

	// Agent Beta sets its own note on the thread
	res, err = sessionB.CallTool(ctx, &mcp.CallToolParams{
		Name: "note_set",
		Arguments: map[string]any{
			"thread": threadRootID,
			"text":   "Beta initial task: write unit tests.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("note_set for Beta failed: %+v", res)
	}

	// 3. Test implicit thread from X-Colonyhall-Thread header
	sessionAWithHeader, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agentAToken, headerTripper{
		headers: map[string]string{"X-Colonyhall-Thread": threadRootID},
		base:    http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sessionAWithHeader.Close() }()

	// note_get without thread argument should pick up thread from header
	res, err = sessionAWithHeader.CallTool(ctx, &mcp.CallToolParams{
		Name:      "note_get",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("note_get with header failed: %+v", res)
	}
	if textContent, ok := res.Content[0].(*mcp.TextContent); !ok || !strings.Contains(textContent.Text, "Alpha initial task") {
		t.Fatalf("unexpected content from header-based note_get: %+v", res.Content)
	}

	// note_set without thread argument updates Alpha's note
	res, err = sessionAWithHeader.CallTool(ctx, &mcp.CallToolParams{
		Name: "note_set",
		Arguments: map[string]any{
			"text": "Alpha updated task: bug located in parser.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("note_set with header failed: %+v", res)
	}

	// 4. Input validation: text too long (> 2000 chars)
	res, err = sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "note_set",
		Arguments: map[string]any{
			"thread": threadRootID,
			"text":   strings.Repeat("a", 2001),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected error for note_set > 2000 chars, got success")
	}

	// 5. REST endpoint tests
	// Human (admin) can list Alpha's notes
	var alphaNotes apispec.AgentNotesResponse
	status := e.do(http.MethodGet, "/api/v1/agents/"+agentAID+"/notes", server.DevAdminToken, nil, &alphaNotes)
	if status != http.StatusOK {
		t.Fatalf("human reading agent notes returned status %d", status)
	}
	if len(alphaNotes.Notes) != 2 {
		t.Fatalf("expected 2 note revisions for Alpha, got %d", len(alphaNotes.Notes))
	}
	// Newest first
	if alphaNotes.Notes[0].Text != "Alpha updated task: bug located in parser." {
		t.Fatalf("expected newest note first, got %q", alphaNotes.Notes[0].Text)
	}

	// Human can filter by thread
	status = e.do(http.MethodGet, "/api/v1/agents/"+agentAID+"/notes?thread="+threadRootID, server.DevAdminToken, nil, &alphaNotes)
	if status != http.StatusOK {
		t.Fatalf("human filtering agent notes by thread returned status %d", status)
	}
	if len(alphaNotes.Notes) != 2 {
		t.Fatalf("expected 2 note revisions with thread filter, got %d", len(alphaNotes.Notes))
	}

	// Agent Alpha can read its own notes via REST
	status = e.do(http.MethodGet, "/api/v1/agents/"+agentAID+"/notes", agentAToken, nil, &alphaNotes)
	if status != http.StatusOK {
		t.Fatalf("agent reading own notes returned status %d", status)
	}
	if len(alphaNotes.Notes) != 2 {
		t.Fatalf("expected 2 note revisions for Alpha, got %d", len(alphaNotes.Notes))
	}

	// Human can also read Beta's notes
	var betaNotes apispec.AgentNotesResponse
	status = e.do(http.MethodGet, "/api/v1/agents/"+agentBID+"/notes", server.DevAdminToken, nil, &betaNotes)
	if status != http.StatusOK {
		t.Fatalf("human reading beta notes returned status %d", status)
	}
	if len(betaNotes.Notes) != 1 || betaNotes.Notes[0].Text != "Beta initial task: write unit tests." {
		t.Fatalf("expected 1 note for Beta, got %+v", betaNotes.Notes)
	}

	// Agent Beta CANNOT read Agent Alpha's notes via REST -> 403 Forbidden
	status = e.do(http.MethodGet, "/api/v1/agents/"+agentAID+"/notes", agentBToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 when Agent Beta reads Agent Alpha's notes, got %d", status)
	}
}

type headerTripper struct {
	headers map[string]string
	base    http.RoundTripper
}

func (h headerTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	req := r.Clone(r.Context())
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}
