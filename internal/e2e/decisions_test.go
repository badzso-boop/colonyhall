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

func TestDecisionsE2E(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	general := e.generalID()

	// Post a message in #general to establish a thread
	msg, s := e.send(server.DevAdminToken, general, map[string]any{
		"body": "Let's decide on the cache architecture",
	})
	if s != http.StatusCreated {
		t.Fatalf("create thread root message: %d", s)
	}
	threadRootID := msg.ID

	// Create an agent
	_, agentToken := e.createAgent("decider-bot", "Decider Bot")

	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", agentToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	// 1. Initial decision_list on empty state
	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "decision_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("decision_list failed: %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "no decisions found") {
		t.Fatalf("expected 'no decisions found', got: %v", tc.Text)
	}

	// 2. Add decision D-1
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "decision_add",
		Arguments: map[string]any{
			"text":      "Use Redis for session caching",
			"rationale": "High throughput and low latency in-memory data store",
			"thread":    threadRootID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("decision_add failed: %+v", res)
	}
	tc, ok = res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "D-1" {
		t.Fatalf("expected D-1, got %q", tc.Text)
	}

	// Verify "Döntések" wiki page was created and contains the decision section
	var wikiPage apispec.WikiPage
	status := e.do(http.MethodGet, "/api/v1/wiki/dontesek", server.DevAdminToken, nil, &wikiPage)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET /api/v1/wiki/dontesek, got %d", status)
	}
	if !strings.Contains(wikiPage.Body, "D-1 — Use Redis for session caching. Why: High throughput and low latency in-memory data store.") {
		t.Fatalf("wiki body missing decision text: %s", wikiPage.Body)
	}
	if !strings.Contains(wikiPage.Body, "@decider-bot") {
		t.Fatalf("wiki body missing author handle: %s", wikiPage.Body)
	}

	// 3. Validation: length limits
	// text > 300 chars
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "decision_add",
		Arguments: map[string]any{
			"text":      strings.Repeat("a", 301),
			"rationale": "Valid rationale",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected error for text > 300 chars")
	}

	// rationale > 500 chars
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "decision_add",
		Arguments: map[string]any{
			"text":      "Valid text",
			"rationale": strings.Repeat("b", 501),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected error for rationale > 500 chars")
	}

	// Superseding non-existent decision
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "decision_add",
		Arguments: map[string]any{
			"text":       "Valid text",
			"rationale":  "Valid rationale",
			"supersedes": 999,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected error for superseding non-existent decision")
	}

	// 4. Add decision D-2 that supersedes D-1 (using "D-1" string format)
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "decision_add",
		Arguments: map[string]any{
			"text":       "Use Dragonfly instead of Redis",
			"rationale":  "Drop-in redis replacement with 25x throughput and multithreading",
			"supersedes": "D-1",
			"thread":     threadRootID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("decision_add for D-2 failed: %+v", res)
	}
	tc, ok = res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "D-2" {
		t.Fatalf("expected D-2, got %q", tc.Text)
	}

	// Verify wiki page now has D-2 and indicates "(supersedes D-1)"
	status = e.do(http.MethodGet, "/api/v1/wiki/dontesek", server.DevAdminToken, nil, &wikiPage)
	if status != http.StatusOK {
		t.Fatalf("expected 200 GET /api/v1/wiki/dontesek, got %d", status)
	}
	if !strings.Contains(wikiPage.Body, "D-2 — Use Dragonfly instead of Redis") || !strings.Contains(wikiPage.Body, "(supersedes D-1)") {
		t.Fatalf("wiki body missing D-2 with supersedes: %s", wikiPage.Body)
	}

	// 5. decision_list should return ONLY D-2 because D-1 is superseded
	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "decision_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || len(res.Content) == 0 {
		t.Fatalf("decision_list failed: %+v", res)
	}
	tc, ok = res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %+v", res.Content[0])
	}
	if !strings.Contains(tc.Text, "D-2") {
		t.Fatalf("expected decision_list to contain D-2, got: %s", tc.Text)
	}
	if strings.Contains(tc.Text, "D-1") {
		t.Fatalf("expected superseded D-1 to NOT be in decision_list, got: %s", tc.Text)
	}

	// 6. REST API: GET /api/v1/decisions
	// Human gets both decisions; D-1 has superseded_by == 2
	var decRes apispec.DecisionsResponse
	status = e.do(http.MethodGet, "/api/v1/decisions", server.DevAdminToken, nil, &decRes)
	if status != http.StatusOK {
		t.Fatalf("human GET /api/v1/decisions returned status %d", status)
	}
	if len(decRes.Decisions) != 2 {
		t.Fatalf("expected 2 decisions for human, got %d", len(decRes.Decisions))
	}
	// Newest first: D-2, then D-1
	if decRes.Decisions[0].Number != 2 || decRes.Decisions[1].Number != 1 {
		t.Fatalf("unexpected order: %+v", decRes.Decisions)
	}
	if decRes.Decisions[1].SupersededBy == nil || *decRes.Decisions[1].SupersededBy != 2 {
		t.Fatalf("expected D-1 to show superseded_by == 2, got: %+v", decRes.Decisions[1].SupersededBy)
	}

	// Agent calling GET /api/v1/decisions -> 403 Forbidden (humans only)
	status = e.do(http.MethodGet, "/api/v1/decisions", agentToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 when agent calls GET /api/v1/decisions, got %d", status)
	}
}
