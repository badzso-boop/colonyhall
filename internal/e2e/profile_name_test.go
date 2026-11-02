package e2e

import (
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// A person renames themselves; an admin can rename anyone. Nobody else can,
// and a blank or too long name is refused.
func TestProfileDisplayName(t *testing.T) {
	e := newEnv(t)
	annaID, annaTok := e.createUser("anna", "Anna")
	bobID, bobTok := e.createUser("bob", "Bob")
	agentID, agentTok := e.createActor("agent", "bot", "member")

	var got apispec.Actor
	if s := e.do("PATCH", "/api/v1/actors/"+annaID+"/profile", annaTok, map[string]any{"display_name": "  Anna K.  "}, &got); s != http.StatusOK {
		t.Fatalf("own rename: %d", s)
	}
	if got.DisplayName != "Anna K." {
		t.Fatalf("name not saved and trimmed: %q", got.DisplayName)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+annaID+"/profile", bobTok, map[string]any{"display_name": "Hacked"}, nil); s != http.StatusForbidden {
		t.Fatalf("another member renamed anna: %d", s)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+agentID+"/profile", agentTok, map[string]any{"display_name": "Boss"}, nil); s != http.StatusForbidden {
		t.Fatalf("an agent renamed itself: %d", s)
	}
	if s := e.do("PATCH", "/api/v1/actors/"+bobID+"/profile", server.DevAdminToken, map[string]any{"display_name": "Robert"}, nil); s != http.StatusOK {
		t.Fatalf("admin rename: %d", s)
	}
	for _, bad := range []string{"   ", "two\nlines", strings.Repeat("x", 81)} {
		if s := e.do("PATCH", "/api/v1/actors/"+annaID+"/profile", annaTok, map[string]any{"display_name": bad}, nil); s != http.StatusBadRequest {
			t.Errorf("name %q accepted: %d", bad, s)
		}
	}
}
