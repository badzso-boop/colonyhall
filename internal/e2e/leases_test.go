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

func TestLeasesE2E(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Create two agents: agent-alpha and agent-bravo
	agentAID, tokenA := e.createAgent("agent-alpha", "Agent Alpha")
	_, tokenB := e.createAgent("agent-bravo", "Agent Bravo")

	sessionA, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", tokenA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sessionA.Close() }()

	sessionB, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", tokenB, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sessionB.Close() }()

	// 2. Agent Alpha acquires lease on "deploy:staging"
	resA, err := sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_acquire",
		Arguments: map[string]any{
			"resource":    "deploy:staging",
			"ttl_minutes": 30,
			"note":        "deploying v1.2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resA.IsError || len(resA.Content) == 0 {
		t.Fatalf("agent A acquire failed: %+v", resA)
	}
	tcA := resA.Content[0].(*mcp.TextContent)
	if !strings.Contains(tcA.Text, "lease on deploy:staging acquired until") {
		t.Fatalf("unexpected acquire result: %s", tcA.Text)
	}

	// 3. Agent Bravo attempts to acquire "deploy:staging" -> returns taken info, no error, no wait
	resB, err := sessionB.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_acquire",
		Arguments: map[string]any{
			"resource":    "deploy:staging",
			"ttl_minutes": 15,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resB.IsError || len(resB.Content) == 0 {
		t.Fatalf("agent B acquire should not error when taken: %+v", resB)
	}
	tcB := resB.Content[0].(*mcp.TextContent)
	if !strings.Contains(tcB.Text, "taken by @agent-alpha until") || !strings.Contains(tcB.Text, "(deploying v1.2)") {
		t.Fatalf("expected taken message with holder and note, got: %s", tcB.Text)
	}

	// 4. Agent Alpha extends its own lease
	resExt, err := sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_acquire",
		Arguments: map[string]any{
			"resource":    "deploy:staging",
			"ttl_minutes": 60,
			"note":        "extending deployment",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resExt.IsError || len(resExt.Content) == 0 {
		t.Fatalf("agent A extend failed: %+v", resExt)
	}
	tcExt := resExt.Content[0].(*mcp.TextContent)
	if !strings.Contains(tcExt.Text, "lease on deploy:staging extended until") {
		t.Fatalf("unexpected extend result: %s", tcExt.Text)
	}

	// 5. Agent Bravo tries to release Alpha's lease -> fails
	resRelB, err := sessionB.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_release",
		Arguments: map[string]any{
			"resource": "deploy:staging",
		},
	})
	if err == nil && !resRelB.IsError {
		t.Fatalf("expected release by non-holder to fail, got: %+v", resRelB)
	}

	// 6. Agent Alpha releases its lease
	resRelA, err := sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_release",
		Arguments: map[string]any{
			"resource": "deploy:staging",
		},
	})
	if err != nil || resRelA.IsError {
		t.Fatalf("agent A release failed: %v, %+v", err, resRelA)
	}
	tcRelA := resRelA.Content[0].(*mcp.TextContent)
	if !strings.Contains(tcRelA.Text, "lease on deploy:staging released") {
		t.Fatalf("unexpected release result: %s", tcRelA.Text)
	}

	// 7. Agent Bravo can now acquire "deploy:staging"
	resB2, err := sessionB.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_acquire",
		Arguments: map[string]any{
			"resource":    "deploy:staging",
			"ttl_minutes": 20,
			"note":        "bravo's turn",
		},
	})
	if err != nil || resB2.IsError {
		t.Fatalf("agent B acquire failed after release: %v, %+v", err, resB2)
	}
	tcB2 := resB2.Content[0].(*mcp.TextContent)
	if !strings.Contains(tcB2.Text, "lease on deploy:staging acquired until") {
		t.Fatalf("unexpected acquire result: %s", tcB2.Text)
	}

	// 8. Human REST API verification
	// Agent GET /api/v1/leases -> 403 Forbidden
	var agentLeases apispec.LeasesResponse
	status := e.do(http.MethodGet, "/api/v1/leases", tokenA, nil, &agentLeases)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for agent GET /api/v1/leases, got %d", status)
	}

	// Human GET /api/v1/leases -> 200 OK with Bravo's active lease
	var humanLeases apispec.LeasesResponse
	status = e.do(http.MethodGet, "/api/v1/leases", server.DevAdminToken, nil, &humanLeases)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for human GET /api/v1/leases, got %d", status)
	}
	if len(humanLeases.Leases) != 1 {
		t.Fatalf("expected 1 active lease, got %d", len(humanLeases.Leases))
	}
	activeL := humanLeases.Leases[0]
	if activeL.Resource != "deploy:staging" || activeL.HolderHandle != "agent-bravo" {
		t.Fatalf("unexpected lease in response: %+v", activeL)
	}

	// Agent POST /api/v1/leases/{id}/release -> 403 Forbidden
	status = e.do(http.MethodPost, "/api/v1/leases/"+activeL.Id.String()+"/release", tokenA, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 for agent release endpoint, got %d", status)
	}

	// Human POST /api/v1/leases/{id}/release -> 200 OK
	var releasedLease apispec.Lease
	status = e.do(http.MethodPost, "/api/v1/leases/"+activeL.Id.String()+"/release", server.DevAdminToken, nil, &releasedLease)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for human release endpoint, got %d", status)
	}
	if releasedLease.ReleasedAt == nil {
		t.Fatalf("expected ReleasedAt to be set, got nil")
	}

	// Check human list is now empty
	status = e.do(http.MethodGet, "/api/v1/leases", server.DevAdminToken, nil, &humanLeases)
	if status != http.StatusOK || len(humanLeases.Leases) != 0 {
		t.Fatalf("expected 0 active leases after release, got %d", len(humanLeases.Leases))
	}

	// 9. Auto-release on agent halt
	resA2, err := sessionA.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_acquire",
		Arguments: map[string]any{
			"resource":    "port:5433",
			"ttl_minutes": 30,
		},
	})
	if err != nil || resA2.IsError {
		t.Fatalf("agent A acquire port:5433 failed: %v, %+v", err, resA2)
	}

	// Stop agent A
	status = e.do(http.MethodPut, "/api/v1/actors/"+agentAID+"/state", server.DevAdminToken, map[string]string{"state": "stopped"}, nil)
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("stop agent A failed: %d", status)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain after halt: %v", err)
	}

	// Now Agent B should be able to acquire port:5433 immediately because A's lease was auto-released
	resB3, err := sessionB.CallTool(ctx, &mcp.CallToolParams{
		Name: "lease_acquire",
		Arguments: map[string]any{
			"resource":    "port:5433",
			"ttl_minutes": 10,
		},
	})
	if err != nil || resB3.IsError {
		t.Fatalf("agent B acquire port:5433 after A halt failed: %v, %+v", err, resB3)
	}
	tcB3 := resB3.Content[0].(*mcp.TextContent)
	if !strings.Contains(tcB3.Text, "lease on port:5433 acquired until") {
		t.Fatalf("expected successful acquisition after halt, got: %s", tcB3.Text)
	}

	// 10. Auto-release on emergency stop
	status = e.do(http.MethodPost, "/api/v1/admin/emergency-stop", server.DevAdminToken, nil, nil)
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("emergency stop failed: %d", status)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatalf("drain after emergency stop: %v", err)
	}

	// Clear emergency stop so we can query normally
	status = e.do(http.MethodPost, "/api/v1/admin/emergency-clear", server.DevAdminToken, map[string]string{"password": server.DevAdminPassword}, nil)
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("clear emergency stop failed: %d", status)
	}

	status = e.do(http.MethodGet, "/api/v1/leases", server.DevAdminToken, nil, &humanLeases)
	if status != http.StatusOK || len(humanLeases.Leases) != 0 {
		t.Fatalf("expected 0 active leases after emergency stop, got %d", len(humanLeases.Leases))
	}
}
