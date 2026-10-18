package e2e

import (
	"context"
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/mcpserver"
	"colonyhall/internal/modules/agentrules"
	"colonyhall/internal/modules/identity"
	"colonyhall/internal/server"
)

func TestAgentRulesE2E(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// Create a non-admin human actor
	owner, err := e.app.Identity.Authenticate(ctx, server.DevAdminToken)
	if err != nil {
		t.Fatal(err)
	}
	member, memberToken, err := e.app.Identity.CreateActor(ctx, owner, identity.NewActorInput{
		Kind:        identity.KindHuman,
		Handle:      "regular-user",
		DisplayName: "Regular User",
		Role:        identity.RoleMember,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Version 0 when empty
	var rulesRes apispec.AgentRulesResponse
	status := e.do(http.MethodGet, "/api/v1/agent-rules", server.DevAdminToken, nil, &rulesRes)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/agent-rules returned status %d", status)
	}
	if rulesRes.Version != 0 || rulesRes.Text != agentrules.DefaultRulesText {
		t.Fatalf("expected version 0 and default text, got version %d", rulesRes.Version)
	}
	if len(rulesRes.History) != 0 {
		t.Fatalf("expected empty history initially, got %d items", len(rulesRes.History))
	}

	// Agent calling GET /api/v1/agent-rules -> 403 Forbidden
	status = e.do(http.MethodGet, "/api/v1/agent-rules", server.DevAgentToken, nil, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 when agent calls GET /api/v1/agent-rules, got %d", status)
	}

	// 2. PUT by non-admin human -> 403
	updateReq := apispec.UpdateAgentRulesRequest{
		Text: "Hello @{handle} from {workspace}. Admins: {admins}.",
	}
	status = e.do(http.MethodPut, "/api/v1/agent-rules", memberToken, updateReq, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 when non-admin calls PUT /api/v1/agent-rules, got %d", status)
	}

	// 3. PUT by agent -> 403
	status = e.do(http.MethodPut, "/api/v1/agent-rules", server.DevAgentToken, updateReq, nil)
	if status != http.StatusForbidden {
		t.Fatalf("expected 403 when agent calls PUT /api/v1/agent-rules, got %d", status)
	}

	// 4. PUT by admin human -> 200 OK, returns updated rules (version 1)
	var updatedRules apispec.AgentRules
	status = e.do(http.MethodPut, "/api/v1/agent-rules", server.DevAdminToken, updateReq, &updatedRules)
	if status != http.StatusOK {
		t.Fatalf("expected 200 when admin calls PUT /api/v1/agent-rules, got %d", status)
	}
	if updatedRules.Version != 1 || updatedRules.Text != updateReq.Text {
		t.Fatalf("unexpected updated rules: %+v", updatedRules)
	}

	// 5. GET /api/v1/agent-rules after update
	status = e.do(http.MethodGet, "/api/v1/agent-rules", server.DevAdminToken, nil, &rulesRes)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/agent-rules returned status %d", status)
	}
	if rulesRes.Version != 1 || rulesRes.Text != updateReq.Text {
		t.Fatalf("expected version 1, got %d", rulesRes.Version)
	}
	if len(rulesRes.History) != 1 || rulesRes.History[0].Version != 1 {
		t.Fatalf("expected history with version 1, got %+v", rulesRes.History)
	}

	// 6. GET historical versions
	var v0Rules apispec.AgentRules
	status = e.do(http.MethodGet, "/api/v1/agent-rules/0", server.DevAdminToken, nil, &v0Rules)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/agent-rules/0 returned %d", status)
	}
	if v0Rules.Version != 0 || v0Rules.Text != agentrules.DefaultRulesText {
		t.Fatalf("unexpected v0: %+v", v0Rules)
	}

	var v1Rules apispec.AgentRules
	status = e.do(http.MethodGet, "/api/v1/agent-rules/1", server.DevAdminToken, nil, &v1Rules)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/agent-rules/1 returned %d", status)
	}
	if v1Rules.Version != 1 || v1Rules.Text != updateReq.Text {
		t.Fatalf("unexpected v1: %+v", v1Rules)
	}

	status = e.do(http.MethodGet, "/api/v1/agent-rules/99", server.DevAdminToken, nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent version, got %d", status)
	}

	// 7. GET /api/v1/agent/instructions with agent token
	var instr apispec.AgentInstructions
	status = e.do(http.MethodGet, "/api/v1/agent/instructions", server.DevAgentToken, nil, &instr)
	if status != http.StatusOK {
		t.Fatalf("GET /api/v1/agent/instructions returned %d", status)
	}
	if instr.Version != 1 {
		t.Fatalf("expected instructions version 1, got %d", instr.Version)
	}
	expectedInstr := "Hello @fake-agent from colonyhall. Admins: @norbi."
	if instr.Text != expectedInstr {
		t.Fatalf("rendered instructions mismatch:\ngot:  %q\nwant: %q", instr.Text, expectedInstr)
	}

	// 8. MCP connect with agent token carries rendered instructions
	agentSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, nil)
	if err != nil {
		t.Fatalf("fakeagent.Connect: %v", err)
	}
	defer func() { _ = agentSession.Close() }()

	if agentSession.InitializeResult().Instructions != expectedInstr {
		t.Fatalf("MCP agent session instructions mismatch:\ngot:  %q\nwant: %q",
			agentSession.InitializeResult().Instructions, expectedInstr)
	}

	// 9. MCP connect with human token falls back to 4-line Instructions
	humanSession, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAdminToken, nil)
	if err != nil {
		t.Fatalf("fakeagent.Connect human: %v", err)
	}
	defer func() { _ = humanSession.Close() }()

	if humanSession.InitializeResult().Instructions != mcpserver.Instructions {
		t.Fatalf("MCP human session instructions mismatch:\ngot:  %q\nwant: %q",
			humanSession.InitializeResult().Instructions, mcpserver.Instructions)
	}

	_ = member
}
