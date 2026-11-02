package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

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

// uiSocket opens /ws/ui as the token's actor and reads up to replay_done.
func uiSocket(ctx context.Context, e *env, token string) *websocket.Conn {
	e.t.Helper()
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", nil)
	if err != nil {
		e.t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	e.t.Cleanup(func() { _ = ws.CloseNow() })
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello", "token": token}); err != nil {
		e.t.Fatal(err)
	}
	var frame map[string]any
	if err := wsjson.Read(ctx, ws, &frame); err != nil || frame["type"] != "welcome" {
		e.t.Fatalf("welcome: %v %v", frame, err)
	}
	return ws
}

// closesSoon reads frames until the server closes the socket.
func closesSoon(ctx context.Context, e *env, ws *websocket.Conn) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		var frame map[string]any
		if err := wsjson.Read(ctx, ws, &frame); err != nil {
			if ctx.Err() != nil {
				e.t.Fatal("the socket stayed open")
			}
			return
		}
	}
}

func TestAccessChangeClosesTheSocket(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	owner := server.DevAdminToken

	adminID, adminTok := e.createActor("human", "boss", "admin")
	ws := uiSocket(ctx, e, adminTok)
	if st := e.do("PUT", "/api/v1/actors/"+adminID+"/role", owner, map[string]any{"role": "member"}, nil); st/100 != 2 {
		t.Fatalf("demote: %d", st)
	}
	closesSoon(ctx, e, ws)

	// Reconnecting with the old cursor must not close it again.
	again := uiSocket(ctx, e, adminTok)
	readCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	for {
		var frame map[string]any
		if err := wsjson.Read(readCtx, again, &frame); err != nil {
			if readCtx.Err() == nil {
				t.Fatalf("a fresh connection was closed: %v", err)
			}
			break
		}
	}

	memberID, memberTok := e.createActor("human", "anna", "member")
	ws = uiSocket(ctx, e, memberTok)
	if st := e.do("PUT", "/api/v1/actors/"+memberID+"/state", owner, map[string]any{"state": "paused"}, nil); st/100 != 2 {
		t.Fatalf("pause: %d", st)
	}
	if st := e.do("PUT", "/api/v1/actors/"+memberID+"/state", owner, map[string]any{"state": "stopped"}, nil); st/100 != 2 {
		t.Fatalf("stop: %d", st)
	}
	closesSoon(ctx, e, ws)

	// The password reset revokes the sessions in the same step as the change.
	resetID, resetTok := e.createUser("zoli", "Zoli")
	ws = uiSocket(ctx, e, resetTok)
	if st := e.do("PUT", "/api/v1/actors/"+resetID+"/password", owner, map[string]any{"new_password": "a-long-new-password-1"}, nil); st/100 != 2 {
		t.Fatalf("reset: %d", st)
	}
	closesSoon(ctx, e, ws)
}

// A person issues their own tokens (the owner key rotation needs it); an agent does not.
func TestOwnTokens(t *testing.T) {
	e := newEnv(t)
	var me apispec.Actor
	e.do("GET", "/api/v1/me", server.DevAdminToken, nil, &me)
	var issued apispec.IssueTokenResponse
	if st := e.do("POST", "/api/v1/actors/"+me.Id.String()+"/tokens", server.DevAdminToken, map[string]any{"label": "new-key"}, &issued); st != http.StatusCreated {
		t.Fatalf("own token: %d", st)
	}
	var viaNew apispec.Actor
	if st := e.do("GET", "/api/v1/me", issued.Secret, nil, &viaNew); st != http.StatusOK || viaNew.Id != me.Id {
		t.Fatalf("the new key does not work: %d", st)
	}
	if st := e.do("DELETE", "/api/v1/tokens/"+issued.Token.Id.String(), server.DevAdminToken, nil, nil); st != http.StatusNoContent {
		t.Fatalf("revoke: %d", st)
	}
	if st := e.do("GET", "/api/v1/me", issued.Secret, nil, &viaNew); st != http.StatusUnauthorized {
		t.Fatalf("the revoked key still works: %d", st)
	}

	memberID, memberTok := e.createActor("human", "anna", "member")
	if st := e.do("POST", "/api/v1/actors/"+memberID+"/tokens", memberTok, map[string]any{"label": "mine"}, nil); st != http.StatusCreated {
		t.Fatalf("a member's own token: %d", st)
	}
	if st := e.do("POST", "/api/v1/actors/"+me.Id.String()+"/tokens", memberTok, map[string]any{"label": "theirs"}, nil); st != http.StatusForbidden {
		t.Fatalf("a member made a token for the owner: %d", st)
	}
	agentID, agentTok := e.createActor("agent", "bot", "member")
	if st := e.do("POST", "/api/v1/actors/"+agentID+"/tokens", agentTok, map[string]any{"label": "self"}, nil); st != http.StatusForbidden {
		t.Fatalf("an agent made its own token: %d", st)
	}
}

// Guessing the password to release the emergency stop runs into a limit.
func TestEmergencyClearIsRateLimited(t *testing.T) {
	e := newEnv(t)
	owner := server.DevAdminToken
	if st := e.do("POST", "/api/v1/admin/emergency-stop", owner, nil, nil); st != http.StatusOK {
		t.Fatalf("stop: %d", st)
	}
	for i := range 5 {
		if st := e.do("POST", "/api/v1/admin/emergency-clear", owner, map[string]any{"password": "wrong-password-here"}, nil); st != http.StatusUnauthorized {
			t.Fatalf("try %d: %d", i, st)
		}
	}
	if st := e.do("POST", "/api/v1/admin/emergency-clear", owner, map[string]any{"password": server.DevAdminPassword}, nil); st != http.StatusTooManyRequests {
		t.Fatalf("over the limit even the right password waits: %d", st)
	}
	e.clock.Advance(16 * time.Minute)
	if st := e.do("POST", "/api/v1/admin/emergency-clear", owner, map[string]any{"password": server.DevAdminPassword}, nil); st != http.StatusOK {
		t.Fatalf("after the window: %d", st)
	}
}

// Revoking the token a socket authenticated with closes it; another token's does not.
func TestRevokedTokenClosesItsSocket(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	memberID, memberTok := e.createActor("human", "anna", "member")
	var second apispec.IssueTokenResponse
	if st := e.do("POST", "/api/v1/actors/"+memberID+"/tokens", memberTok, map[string]any{"label": "second"}, &second); st != http.StatusCreated {
		t.Fatalf("issue: %d", st)
	}
	staying := uiSocket(ctx, e, memberTok)
	going := uiSocket(ctx, e, second.Secret)
	if st := e.do("DELETE", "/api/v1/tokens/"+second.Token.Id.String(), server.DevAdminToken, nil, nil); st != http.StatusNoContent {
		t.Fatalf("revoke: %d", st)
	}
	closesSoon(ctx, e, going)
	readCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	for {
		var frame map[string]any
		if err := wsjson.Read(readCtx, staying, &frame); err != nil {
			if readCtx.Err() == nil {
				t.Fatalf("the other token's socket closed: %v", err)
			}
			return
		}
	}
}

// A deleted agent loses its socket.
func TestArchivedAgentLosesItsSocket(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	var me apispec.Actor
	e.do("GET", "/api/v1/me", server.DevAgentToken, nil, &me)
	ws := uiSocket(ctx, e, server.DevAgentToken)
	if st := e.do("DELETE", "/api/v1/agents/"+me.Id.String(), server.DevAdminToken, nil, nil); st/100 != 2 {
		t.Fatalf("delete: %d", st)
	}
	closesSoon(ctx, e, ws)
}

// A logout leaves no event: the periodic re-check closes that browser's socket.
func TestLogoutClosesTheSocketSoon(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	e.app.Gateway.SetRevalidateEvery(50 * time.Millisecond)
	b := e.browser()
	if st := b.login("norbi", server.DevAdminPassword); st != http.StatusOK {
		t.Fatalf("login: %d", st)
	}
	ws, res, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws/ui", &websocket.DialOptions{HTTPClient: b.client})
	if err != nil {
		t.Fatal(err)
	}
	if res.Body != nil {
		_ = res.Body.Close()
	}
	t.Cleanup(func() { _ = ws.CloseNow() })
	if err := wsjson.Write(ctx, ws, map[string]any{"type": "hello"}); err != nil {
		t.Fatal(err)
	}
	var frame map[string]any
	if err := wsjson.Read(ctx, ws, &frame); err != nil || frame["type"] != "welcome" {
		t.Fatalf("welcome: %v %v", frame, err)
	}
	// A cancelled read closes the socket, so one reader runs until the server closes it.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			var f map[string]any
			if err := wsjson.Read(ctx, ws, &f); err != nil {
				return
			}
		}
	}()
	select {
	case <-closed:
		t.Fatal("closed while logged in")
	case <-time.After(250 * time.Millisecond):
	}
	if st := b.do("POST", "/api/v1/auth/logout", nil, nil); st/100 != 2 {
		t.Fatalf("logout: %d", st)
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the socket stayed open after the logout")
	}
}
