package e2e

import (
	"net/http"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// Round 55, part G: a thread reply "also sent to #channel" is one message
// with a flag, shown in the main stream; channel agents still wake only
// when mentioned.
func TestAlsoInChannel(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "release notes?"})
	before := len(e.inbox(server.DevAgentToken))

	reply, s := e.send(server.DevAdminToken, general, map[string]any{"body": "done, see the wiki", "thread_root_id": root.ID, "also_in_channel": true})
	if s != http.StatusCreated {
		t.Fatalf("send: %d", s)
	}
	var found *apispec.Message
	count := 0
	for _, m := range e.getMessages(general) {
		if m.Id.String() == reply.ID {
			count++
			found = &m
		}
	}
	if count != 1 || found.AlsoInChannel == nil || !*found.AlsoInChannel || found.ThreadRootId == nil || found.ThreadRootId.String() != root.ID {
		t.Fatalf("one flagged reply in the channel: %d %+v", count, found)
	}
	if after := len(e.inbox(server.DevAgentToken)); after != before {
		t.Fatalf("a channel agent was woken without a mention (%d → %d)", before, after)
	}
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent please check", "thread_root_id": root.ID, "also_in_channel": true}); s != http.StatusCreated {
		t.Fatalf("send with mention: %d", s)
	}
	if after := len(e.inbox(server.DevAgentToken)); after != before+1 {
		t.Fatalf("a mentioned agent was not woken (%d → %d)", before, after)
	}

	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "x", "also_in_channel": true}); s != http.StatusBadRequest {
		t.Fatalf("also_in_channel without a thread: %d", s)
	}
	var dm apispec.Channel
	e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{e.agentID()}}, &dm)
	dmRoot, _ := e.send(server.DevAdminToken, dm.Id.String(), map[string]any{"body": "x"})
	if _, s := e.send(server.DevAdminToken, dm.Id.String(), map[string]any{"body": "y", "thread_root_id": dmRoot.ID, "also_in_channel": true}); s != http.StatusBadRequest {
		t.Fatalf("also_in_channel in a DM: %d", s)
	}
}
