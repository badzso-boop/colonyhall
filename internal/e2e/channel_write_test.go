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

// setCanWrite switches a member's write permission as the admin.
func (e *env) setCanWrite(channelID, actorID string, canWrite bool) apispec.ChannelMember {
	e.t.Helper()
	var m apispec.ChannelMember
	if s := e.do("PATCH", "/api/v1/channels/"+channelID+"/members/"+actorID, server.DevAdminToken,
		map[string]any{"can_write": canWrite}, &m); s != http.StatusOK {
		e.t.Fatalf("PATCH member can_write=%v: %d", canWrite, s)
	}
	return m
}

func (e *env) addMember(channelID, actorID string, canWrite bool) {
	e.t.Helper()
	if s := e.do("POST", "/api/v1/channels/"+channelID+"/members", server.DevAdminToken,
		map[string]any{"actor_id": actorID, "can_write": canWrite}, nil); s != http.StatusNoContent {
		e.t.Fatalf("add member: %d", s)
	}
}

func (e *env) memberCanWrite(channelID, actorID string) bool {
	e.t.Helper()
	var list apispec.ChannelMemberList
	if s := e.do("GET", "/api/v1/channels/"+channelID+"/members", server.DevAdminToken, nil, &list); s != http.StatusOK {
		e.t.Fatalf("list members: %d", s)
	}
	for _, m := range list.Members {
		if m.ActorId.String() == actorID {
			return m.CanWrite
		}
	}
	e.t.Fatalf("%s is not a member", actorID)
	return false
}

// toolFails calls an MCP tool and reports whether it was refused, with the
// error text.
func toolFails(ctx context.Context, t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (bool, string) {
	t.Helper()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return true, err.Error()
	}
	if res.IsError {
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return true, b.String()
	}
	return false, ""
}

func TestChannelMemberWritePermissionSwitch(t *testing.T) {
	e := newEnv(t)
	annaID, annaTok := e.createActor("human", "anna", "member")
	ro := e.createChannel("readonly-room")
	e.addMember(ro, annaID, true)

	own, s := e.send(annaTok, ro, map[string]any{"body": "még írhatok"})
	if s != http.StatusCreated {
		t.Fatalf("writable member send: %d", s)
	}
	root, s := e.send(server.DevAdminToken, ro, map[string]any{"body": "szál indul"})
	if s != http.StatusCreated {
		t.Fatalf("admin send: %d", s)
	}

	// Only admins change it; the body must say what it wants.
	if s := e.do("PATCH", "/api/v1/channels/"+ro+"/members/"+annaID, annaTok, map[string]any{"can_write": true}, nil); s != http.StatusForbidden {
		t.Fatalf("member PATCH: want 403, got %d", s)
	}
	if s := e.do("PATCH", "/api/v1/channels/"+ro+"/members/"+annaID, server.DevAdminToken, map[string]any{}, nil); s != http.StatusBadRequest {
		t.Fatalf("PATCH without can_write: want 400, got %d", s)
	}
	outsiderID, _ := e.createActor("human", "kivul", "member")
	if s := e.do("PATCH", "/api/v1/channels/"+ro+"/members/"+outsiderID, server.DevAdminToken, map[string]any{"can_write": false}, nil); s != http.StatusNotFound {
		t.Fatalf("PATCH a non-member: want 404, got %d", s)
	}
	var dm apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{annaID}}, &dm); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}
	if s := e.do("PATCH", "/api/v1/channels/"+dm.Id.String()+"/members/"+annaID, server.DevAdminToken, map[string]any{"can_write": false}, nil); s != http.StatusConflict {
		t.Fatalf("PATCH a DM member: want 409, got %d", s)
	}

	m := e.setCanWrite(ro, annaID, false)
	if m.CanWrite || m.Handle != "anna" {
		t.Fatalf("PATCH answer: %+v", m)
	}
	if e.memberCanWrite(ro, annaID) {
		t.Fatal("member list still says can_write")
	}
	// Setting the same value again changes nothing (no second note).
	e.setCanWrite(ro, annaID, false)

	// Every write path refuses a read-only member; reading still works.
	if _, s := e.send(annaTok, ro, map[string]any{"body": "nem megy"}); s != http.StatusForbidden {
		t.Fatalf("read-only message: want 403, got %d", s)
	}
	if _, s := e.send(annaTok, ro, map[string]any{"body": "válasz", "thread_root_id": root.ID}); s != http.StatusForbidden {
		t.Fatalf("read-only thread reply: want 403, got %d", s)
	}
	if s := e.do("PUT", "/api/v1/messages/"+root.ID+"/reactions/👍", annaTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("read-only reaction: want 403, got %d", s)
	}
	if s, _ := uploadMultipart(t, e, ro, "a.txt", []byte("hello"), annaTok); s != http.StatusForbidden {
		t.Fatalf("read-only upload: want 403, got %d", s)
	}
	if s := e.do("POST", "/api/v1/messages/"+own.ID+"/retract", annaTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("read-only retract: want 403, got %d", s)
	}
	at := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if s := e.do("POST", "/api/v1/schedules", annaTok, map[string]any{"channel_id": ro, "body": "emlékeztető", "at": at}, nil); s != http.StatusForbidden {
		t.Fatalf("read-only schedule: want 403, got %d", s)
	}
	var list apispec.MessageList
	if s := e.do("GET", "/api/v1/channels/"+ro+"/messages", annaTok, nil, &list); s != http.StatusOK {
		t.Fatalf("read-only member reads: %d", s)
	}

	// Switching back works without re-adding.
	if m := e.setCanWrite(ro, annaID, true); !m.CanWrite {
		t.Fatalf("PATCH back: %+v", m)
	}
	if _, s := e.send(annaTok, ro, map[string]any{"body": "újra írhatok"}); s != http.StatusCreated {
		t.Fatalf("send after switching back: %d", s)
	}
	if s := e.do("POST", "/api/v1/schedules", annaTok, map[string]any{"channel_id": ro, "body": "emlékeztető", "at": at}, nil); s != http.StatusCreated {
		t.Fatalf("schedule after switching back: %d", s)
	}

	notes := e.systemBodies(ro)
	var ro1, rw int
	for _, n := range notes {
		if strings.Contains(n, "@anna mostantól csak olvashatja a csatornát") {
			ro1++
		}
		if strings.Contains(n, "@anna mostantól írhatja a csatornát") {
			rw++
		}
	}
	if ro1 != 1 || rw != 1 {
		t.Fatalf("silent notes: %v", notes)
	}

	if err := e.app.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	var updates int
	for _, row := range auditPage(e, server.DevAdminToken, "action=channel.member_updated").Entries {
		if row.Payload["actor_id"] == annaID {
			updates++
		}
	}
	if updates != 2 {
		t.Fatalf("audit: want 2 channel.member_updated rows, got %d", updates)
	}
}

func TestReadOnlyHumanCannotRequestFileFetch(t *testing.T) {
	e := newEnv(t)
	annaID, annaTok := e.createActor("human", "anna", "member")
	agent := e.agentID()
	ro := e.createChannel("fetch-room")
	e.addMember(ro, annaID, false)
	e.addMember(ro, agent, true)
	msg, s := e.send(server.DevAgentToken, ro, map[string]any{"body": "Kész: `/app/notes.txt`"})
	if s != http.StatusCreated {
		t.Fatalf("agent send: %d", s)
	}

	fetch := func() int {
		return e.do("POST", "/api/v1/messages/"+msg.ID+"/file-fetch", annaTok, map[string]any{"path": "/app/notes.txt"}, nil)
	}
	agentFetch := func() int {
		return e.do("POST", "/api/v1/agents/"+agent+"/file-fetch", annaTok, map[string]any{"channel_id": ro, "path": "/app/notes.txt"}, nil)
	}
	if s := fetch(); s != http.StatusForbidden {
		t.Fatalf("read-only file fetch from a message: want 403, got %d", s)
	}
	if s := agentFetch(); s != http.StatusForbidden {
		t.Fatalf("read-only file fetch from an agent folder: want 403, got %d", s)
	}
	// With write permission the permission check passes (the agent has no
	// machine here, so the request fails further on, not with 403).
	e.setCanWrite(ro, annaID, true)
	if s := fetch(); s == http.StatusForbidden {
		t.Fatalf("writable file fetch from a message: got 403")
	}
	if s := agentFetch(); s == http.StatusForbidden {
		t.Fatalf("writable file fetch from an agent folder: got 403")
	}
}

func TestReadOnlyHumanCannotDecideApproval(t *testing.T) {
	e := newEnv(t)
	annaID, annaTok := e.createActor("human", "anna", "member")
	ro := e.createChannel("approval-room")
	e.addMember(ro, annaID, false)
	e.addMember(ro, e.agentID(), true)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, &headersTransport{
		headers: map[string]string{"X-Colonyhall-Channel": ro},
		base:    http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	done := make(chan struct{})
	go func() {
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{Name: "approve", Arguments: map[string]any{
			"tool_name": "Bash", "input": map[string]any{"command": "ls"},
		}})
		close(done)
	}()
	appID, _ := findApprovalInMessages(t, e, ro)
	if s := e.do("POST", "/api/v1/approvals/"+appID, annaTok, map[string]any{"decision": "allow"}, nil); s != http.StatusForbidden {
		t.Fatalf("read-only member decides: want 403, got %d", s)
	}
	if s := e.do("POST", "/api/v1/approvals/"+appID, server.DevAdminToken, map[string]any{"decision": "deny"}, nil); s != http.StatusOK {
		t.Fatalf("admin decides: %d", s)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("approve tool did not return")
	}
}

func TestReadOnlyAgentCannotAct(t *testing.T) {
	e := newEnv(t)
	agent := e.agentID()
	ro := e.createChannel("agent-ro")
	e.addMember(ro, agent, true)
	root, s := e.send(server.DevAgentToken, ro, map[string]any{"body": "Ezen dolgozom."})
	if s != http.StatusCreated {
		t.Fatalf("writable agent send: %d", s)
	}
	hostID, _ := e.enroll("dev-box")
	e.configure(agent, map[string]any{
		"host_id": hostID, "client": "claude-code", "model": "haiku",
		"allow_work": true, "root_dir": "/app", "allow_preview": true,
	})
	e.setCanWrite(ro, agent, false)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// REST paths of the agent.
	if _, s := e.send(server.DevAgentToken, ro, map[string]any{"body": "nem megy"}); s != http.StatusForbidden {
		t.Fatalf("read-only agent message: want 403, got %d", s)
	}
	if s := e.do("POST", "/api/v1/agent/terminal-start", server.DevAgentToken, map[string]any{"channel_id": ro, "name": "build"}, nil); s != http.StatusForbidden {
		t.Fatalf("read-only terminal start: want 403, got %d", s)
	}
	if s := e.do("POST", "/api/v1/agent/terminal-events", server.DevAgentToken, map[string]any{
		"name": "build", "channel_id": ro, "duration_ms": 10, "tail": "ok",
	}, nil); s != http.StatusForbidden {
		t.Fatalf("read-only terminal event: want 403, got %d", s)
	}

	// MCP tools bound to the channel.
	session, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", server.DevAgentToken, &headersTransport{
		headers: map[string]string{"X-Colonyhall-Channel": ro},
		base:    http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	if failed, _ := toolFails(ctx, t, session, "decision_add", map[string]any{"text": "Redis", "rationale": "gyors", "thread": root.ID}); !failed {
		t.Fatal("read-only decision_add succeeded")
	}
	if failed, _ := toolFails(ctx, t, session, "ask", map[string]any{"question": "Mehet?", "channel": "agent-ro"}); !failed {
		t.Fatal("read-only ask succeeded")
	}
	// Refused at once: without the check the request would be stored and
	// the tool would wait for a decision nobody can see in the channel.
	approved := make(chan bool, 1)
	go func() {
		failed, _ := toolFails(ctx, t, session, "approve", map[string]any{"tool_name": "Bash", "input": map[string]any{"command": "ls"}})
		approved <- failed
	}()
	select {
	case failed := <-approved:
		if !failed {
			t.Fatal("read-only approve succeeded")
		}
	case <-time.After(3 * time.Second):
		// Pausing the agent ends the wait, so the server can shut down.
		e.do("PUT", "/api/v1/actors/"+agent+"/state", server.DevAdminToken, map[string]any{"state": "paused"}, nil)
		<-approved
		t.Fatal("read-only approve was stored and waits for a decision")
	}
	if failed, msg := toolFails(ctx, t, session, "preview_start", map[string]any{"name": "web", "compose_path": "docker-compose.yaml", "channel_id": ro}); !failed || strings.Contains(msg, "not enabled") {
		t.Fatalf("read-only preview_start: failed=%v %q", failed, msg)
	}
	if failed, _ := toolFails(ctx, t, session, "react", map[string]any{"message": root.ID, "emoji": "👍"}); !failed {
		t.Fatal("read-only react succeeded")
	}

	// A mention does not give a read-only agent a task.
	inboxFor := func() int {
		if err := e.app.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		var in apispec.Inbox
		if s := e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in); s != http.StatusOK {
			t.Fatalf("inbox: %d", s)
		}
		n := 0
		for _, it := range in.Items {
			if it.ChannelId.String() == ro {
				n++
			}
		}
		return n
	}
	if _, s := e.send(server.DevAdminToken, ro, map[string]any{"body": "@fake-agent nézd meg"}); s != http.StatusCreated {
		t.Fatalf("admin mention: %d", s)
	}
	if n := inboxFor(); n != 0 {
		t.Fatalf("read-only agent was given %d items", n)
	}

	// Switched back to write, the same agent is woken again.
	e.setCanWrite(ro, agent, true)
	if _, s := e.send(server.DevAdminToken, ro, map[string]any{"body": "@fake-agent most már igen"}); s != http.StatusCreated {
		t.Fatalf("admin mention: %d", s)
	}
	if n := inboxFor(); n != 1 {
		t.Fatalf("writable agent: want 1 item, got %d", n)
	}
}
