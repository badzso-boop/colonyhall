// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

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

// Round 40: an admin cannot change any field of the owner's profile (only
// the name was rank-checked before).
func TestAdminCannotEditOwnersProfile(t *testing.T) {
	e := newEnv(t)
	_, adminTok := e.createActor("human", "ada", "admin")
	var me struct{ Id string }
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me)
	for _, body := range []map[string]any{{"description": "fired"}, {"status_text": "away for good"}, {"timezone": "UTC"}, {"avatar_emoji": "🤡"}} {
		if s := e.do("PATCH", "/api/v1/actors/"+me.Id+"/profile", adminTok, body, nil); s != http.StatusForbidden {
			t.Fatalf("an admin changed the owner's %v: %d", body, s)
		}
	}
	memberID, _ := e.createActor("human", "mia", "member")
	if s := e.do("PATCH", "/api/v1/actors/"+memberID+"/profile", adminTok, map[string]any{"description": "QA"}, nil); s != http.StatusOK {
		t.Fatalf("an admin edits a member's profile: %d", s)
	}
}
