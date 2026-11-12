package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/server"
)

// Round 50: a guard-issued agent token is deleted a week after it expired;
// using it afterwards fails exactly as it did while it was only expired.
func TestExpiredHostTokensAreDeleted(t *testing.T) {
	e := newEnv(t)
	host, hostToken := e.enroll("cleanup-host")
	var created struct{ Actor struct{ ID string } }
	if s := e.do("POST", "/api/v1/agents", server.DevAdminToken, map[string]any{"handle": "cleanup-claude",
		"host": map[string]any{"host_id": host, "client": "claude-code", "mode": "compact"}}, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	agent := created.Actor.ID
	var tok struct{ Token string }
	if s := e.do("POST", "/api/v1/host/agents/"+agent+"/token", hostToken, nil, &tok); s != http.StatusCreated {
		t.Fatalf("agent token: %d", s)
	}
	tokenCount := func() int {
		var list struct{ Tokens []json.RawMessage }
		if s := e.do("GET", "/api/v1/actors/"+agent+"/tokens", server.DevAdminToken, nil, &list); s != http.StatusOK {
			t.Fatalf("list tokens: %d", s)
		}
		return len(list.Tokens)
	}
	if n := tokenCount(); n != 1 {
		t.Fatalf("tokens before: %d", n)
	}

	// Expired (the TTL is an hour) but younger than a week: kept, refused.
	e.clock.Advance(2 * time.Hour)
	var expiredErr, deletedErr json.RawMessage
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, &expiredErr); s != http.StatusUnauthorized {
		t.Fatalf("expired token: %d", s)
	}
	e.app.Maintain(context.Background())
	if n := tokenCount(); n != 1 {
		t.Fatalf("a recently expired token must stay: %d", n)
	}

	// Eight days after it expired: deleted, refused the same way.
	e.clock.Advance(8 * 24 * time.Hour)
	e.app.Maintain(context.Background())
	if n := tokenCount(); n != 0 {
		t.Fatalf("an old expired token must go: %d", n)
	}
	if s := e.do("GET", "/api/v1/me", tok.Token, nil, &deletedErr); s != http.StatusUnauthorized {
		t.Fatalf("deleted token: %d", s)
	}
	if string(expiredErr) != string(deletedErr) {
		t.Fatalf("a deleted token must fail like an expired one: %s vs %s", expiredErr, deletedErr)
	}
	// The hand-made dev tokens are untouched.
	if s := e.do("GET", "/api/v1/me", server.DevAgentToken, nil, nil); s != http.StatusOK {
		t.Fatalf("dev agent token: %d", s)
	}
}
