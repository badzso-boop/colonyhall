package e2e

import (
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// Only the owner creates and changes people; an admin cannot make an admin.
func TestUserManagementOwnerOnly(t *testing.T) {
	e := newEnv(t)
	owner := server.DevAdminToken
	var me apispec.Actor
	e.do("GET", "/api/v1/me", owner, nil, &me)
	if me.Role != "owner" {
		t.Fatalf("the dev seed's admin is %s", me.Role)
	}
	adminID, adminTok := e.createActor("human", "boss", "admin")
	memberID, memberTok := e.createUser("anna", "Anna")
	agentID, agentTok := e.createActor("agent", "bot", "member")

	// Creating people, with an initial password.
	create := map[string]any{"kind": "human", "handle": "new-user", "display_name": "New", "role": "member", "password": "a-long-enough-password"}
	for _, tok := range []string{adminTok, memberTok, agentTok} {
		if s := e.do("POST", "/api/v1/actors", tok, create, nil); s != http.StatusForbidden {
			t.Errorf("non-owner created a person: %d", s)
		}
	}
	short := map[string]any{"kind": "human", "handle": "short-pw", "password": "short"}
	if s := e.do("POST", "/api/v1/actors", owner, short, nil); s != http.StatusBadRequest {
		t.Errorf("a weak initial password: %d", s)
	}
	var created apispec.CreateActorResponse
	if s := e.do("POST", "/api/v1/actors", owner, create, &created); s != http.StatusCreated {
		t.Fatalf("create: %d", s)
	}
	if s := e.do("POST", "/api/v1/auth/login", "", map[string]any{"handle": "new-user", "password": "a-long-enough-password"}, nil); s != http.StatusOK {
		t.Errorf("the initial password does not log in: %d", s)
	}

	// Roles.
	role := func(tok, id, r string) int {
		return e.do("PUT", "/api/v1/actors/"+id+"/role", tok, map[string]any{"role": r}, nil)
	}
	if s := role(adminTok, memberID, "admin"); s != http.StatusForbidden {
		t.Errorf("an admin made an admin: %d", s)
	}
	if s := role(adminTok, memberID, "observer"); s != http.StatusForbidden {
		t.Errorf("an admin changed a role: %d", s)
	}
	if s := role(owner, me.Id.String(), "member"); s != http.StatusForbidden {
		t.Errorf("the owner demoted themself: %d", s)
	}
	if s := role(owner, memberID, "owner"); s != http.StatusBadRequest {
		t.Errorf("a second owner: %d", s)
	}
	if s := role(owner, agentID, "admin"); s != http.StatusForbidden {
		t.Errorf("an agent got a role: %d", s)
	}
	if s := role(owner, memberID, "observer"); s != http.StatusNoContent {
		t.Errorf("owner changes a role: %d", s)
	}

	// Disabling.
	state := func(tok, id, st string) int {
		return e.do("PUT", "/api/v1/actors/"+id+"/state", tok, map[string]any{"state": st}, nil)
	}
	if s := state(adminTok, memberID, "stopped"); s != http.StatusForbidden {
		t.Errorf("an admin disabled a person: %d", s)
	}
	if s := state(owner, me.Id.String(), "stopped"); s != http.StatusForbidden {
		t.Errorf("the owner disabled themself: %d", s)
	}
	if s := state(owner, adminID, "stopped"); s != http.StatusNoContent {
		t.Errorf("owner disables: %d", s)
	}
	if s := state(owner, adminID, "active"); s != http.StatusNoContent {
		t.Errorf("owner enables: %d", s)
	}
	// An admin may still pause an agent.
	if s := state(adminTok, agentID, "paused"); s != http.StatusNoContent {
		t.Errorf("an admin pauses an agent: %d", s)
	}

	// Password reset.
	pw := func(tok, id string) int {
		return e.do("PUT", "/api/v1/actors/"+id+"/password", tok, map[string]any{"new_password": "another-long-password"}, nil)
	}
	if s := pw(adminTok, memberID); s != http.StatusForbidden {
		t.Errorf("an admin reset a password: %d", s)
	}
	if s := pw(owner, memberID); s != http.StatusNoContent {
		t.Errorf("owner resets a password: %d", s)
	}
	if s := e.do("POST", "/api/v1/auth/login", "", map[string]any{"handle": "anna", "password": "another-long-password"}, nil); s != http.StatusOK {
		t.Errorf("the reset password does not log in: %d", s)
	}

	// The list.
	for _, tok := range []string{adminTok, memberTok, agentTok} {
		if s := e.do("GET", "/api/v1/admin/users", tok, nil, nil); s != http.StatusForbidden {
			t.Errorf("non-owner listed users: %d", s)
		}
	}
	var users apispec.UserList
	if s := e.do("GET", "/api/v1/admin/users", owner, nil, &users); s != http.StatusOK {
		t.Fatalf("users: %d", s)
	}
	found := false
	for _, u := range users.Users {
		if string(u.Actor.Kind) == "agent" {
			t.Errorf("an agent in the users list: %s", u.Actor.Handle)
		}
		if u.Actor.Handle == "anna" {
			found = u.ActiveSessions == 1 && u.LastLoginAt != nil && u.Actor.Role == "observer"
		}
	}
	if !found {
		t.Errorf("anna: %+v", users.Users)
	}

	// Every change is audited, without the password.
	var audit apispec.AuditPage
	e.do("GET", "/api/v1/admin/audit?limit=200", owner, nil, &audit)
	seen := map[string]bool{}
	if err := e.app.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	e.do("GET", "/api/v1/admin/audit?limit=200", owner, nil, &audit)
	for _, a := range audit.Entries {
		seen[a.Action] = true
	}
	for _, want := range []string{"actor.created", "actor.role_changed", "actor.state_changed", "actor.password_reset"} {
		if !seen[want] {
			t.Errorf("not audited: %s", want)
		}
	}
}
