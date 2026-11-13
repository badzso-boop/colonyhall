package e2e

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"colonyhall/internal/server"
)

// mcpPost sends one JSON-RPC message and returns the status, the session
// header and the body.
func (e *env) mcpPost(token, session, body string) (int, string, string) {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/mcp", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if session != "" {
		req.Header.Set("Mcp-Session-Id", session)
		req.Header.Set("Mcp-Protocol-Version", "2025-06-18")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Mcp-Session-Id"), string(raw)
}

// Round 40: an MCP session belongs to the actor that opened it. Another
// valid token with a leaked Mcp-Session-Id is refused.
func TestMCPSessionIsBoundToItsActor(t *testing.T) {
	e := newEnv(t)
	st, session, body := e.mcpPost(server.DevAgentToken, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if st != http.StatusOK || session == "" {
		t.Fatalf("initialize: %d %s", st, body)
	}
	if st, _, body := e.mcpPost(server.DevAgentToken, session, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); st >= 300 {
		t.Fatalf("initialized: %d %s", st, body)
	}
	list := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	if st, _, body := e.mcpPost(server.DevAgentToken, session, list); st != http.StatusOK {
		t.Fatalf("the owner's own session: %d %s", st, body)
	}
	if st, _, body := e.mcpPost(server.DevAdminToken, session, list); st != http.StatusForbidden {
		t.Fatalf("another actor's token on the session must be refused: %d %s", st, body)
	}
}
