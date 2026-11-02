package e2e

import (
	"net/http"
	"testing"

	"colonyhall/internal/server"
)

// TestAdminCannotTakeOverOwner: an admin may manage lower-ranked people but
// never the owner or another admin, and agents never manage accounts.
func TestAdminCannotTakeOverOwner(t *testing.T) {
	e := newEnv(t)
	var created struct {
		Actor struct{ ID string }
		Token string
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken,
		map[string]any{"kind": "human", "handle": "anna", "role": "admin"}, &created); s != http.StatusCreated {
		t.Fatalf("owner creates admin: status %d", s)
	}
	admin := created.Token
	var owner struct{ ID string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &owner)

	if s := e.do("PUT", "/api/v1/actors/"+owner.ID+"/password", admin,
		map[string]any{"new_password": "attacker-chosen-password"}, nil); s != http.StatusForbidden {
		t.Fatalf("admin resets owner password: status %d", s)
	}
	if s := e.do("POST", "/api/v1/actors", admin,
		map[string]any{"kind": "human", "handle": "mallory", "role": "admin"}, nil); s != http.StatusForbidden {
		t.Fatalf("admin creates another admin: status %d", s)
	}
	// Round 35: only the owner creates people and resets their passwords.
	if s := e.do("POST", "/api/v1/actors", admin,
		map[string]any{"kind": "human", "handle": "bela", "role": "member"}, nil); s != http.StatusForbidden {
		t.Fatalf("admin creates member: status %d", s)
	}
	var member struct {
		Actor struct{ ID string }
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAdminToken,
		map[string]any{"kind": "human", "handle": "bela", "role": "member"}, &member); s != http.StatusCreated {
		t.Fatalf("owner creates member: status %d", s)
	}
	if s := e.do("PUT", "/api/v1/actors/"+member.Actor.ID+"/password", admin,
		map[string]any{"new_password": "a-fine-member-password"}, nil); s != http.StatusForbidden {
		t.Fatalf("admin sets member password: status %d", s)
	}
	if s := e.do("POST", "/api/v1/actors", server.DevAgentToken,
		map[string]any{"kind": "agent", "handle": "spawned"}, nil); s != http.StatusForbidden {
		t.Fatalf("agent creates actor: status %d", s)
	}
}

// TestPasswordResetEndsOtherSessions: after a password change the person's
// other browsers are logged out, but the browser that made the change stays.
func TestPasswordResetEndsOtherSessions(t *testing.T) {
	e := newEnv(t)
	laptop, phone := e.browser(), e.browser()
	laptop.login("norbi", server.DevAdminPassword)
	phone.login("norbi", server.DevAdminPassword)
	var me struct{ ID string }
	laptop.do("GET", "/api/v1/me", nil, &me)
	if s := laptop.do("PUT", "/api/v1/actors/"+me.ID+"/password",
		map[string]any{"current_password": server.DevAdminPassword, "new_password": "rotated-password-123"}, nil); s != http.StatusNoContent {
		t.Fatalf("change: status %d", s)
	}
	if s := laptop.do("GET", "/api/v1/me", nil, nil); s != http.StatusOK {
		t.Fatalf("the changing session must stay: status %d", s)
	}
	if s := phone.do("GET", "/api/v1/me", nil, nil); s != http.StatusUnauthorized {
		t.Fatalf("other session must be logged out: status %d", s)
	}
}
