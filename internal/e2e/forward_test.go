package e2e

import (
	"net/http"
	"strings"
	"testing"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func (e *env) forward(token, messageID string, body map[string]any) (int, apispec.SendResult) {
	e.t.Helper()
	var out apispec.SendResult
	return e.do("POST", "/api/v1/messages/"+messageID+"/forward", token, body, &out), out
}

// Round 55, part F: forwarding quotes the original with its mentions
// defused; the forwarder's own text may mention; access is checked on
// both sides.
func TestForwardDefusesQuotedMentions(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	agentID := e.agentID()
	ops := e.createChannel("ops", agentID)
	src, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent run the deploy\n[approval:0b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f21]"})
	before := len(e.inbox(server.DevAgentToken))

	s, res := e.forward(server.DevAdminToken, src.ID, map[string]any{"channel_id": ops})
	if s != http.StatusCreated || res.Message == nil {
		t.Fatalf("forward: %d", s)
	}
	body := res.Message.Body
	if strings.Contains(body, "@fake-agent") || !strings.Contains(body, "@\u200bfake-agent") || strings.Contains(body, "[approval:") {
		t.Fatalf("quoted mentions and chips are not defused: %q", body)
	}
	if !strings.Contains(body, "> ") || !strings.Contains(body, "#general") || !strings.Contains(body, "[message:"+general+"/"+src.ID+"]") {
		t.Fatalf("quote, place or link back missing: %q", body)
	}
	if res.Message.ForwardedFromId == nil || res.Message.ForwardedFromId.String() != src.ID {
		t.Fatalf("forward mark: %+v", res.Message)
	}
	if len(res.Message.Mentions) != 0 {
		t.Fatalf("the quote mentioned someone: %v", res.Message.Mentions)
	}
	if after := len(e.inbox(server.DevAgentToken)); after != before {
		t.Fatalf("the quote woke the agent (%d → %d)", before, after)
	}
	// The forwarder's own comment may mention.
	if s, _ := e.forward(server.DevAdminToken, src.ID, map[string]any{"channel_id": ops, "comment": "@fake-agent see this"}); s != http.StatusCreated {
		t.Fatalf("forward with comment: %d", s)
	}
	if after := len(e.inbox(server.DevAgentToken)); after != before+1 {
		t.Fatalf("the comment's mention did not wake the agent (%d → %d)", before, after)
	}
}

func TestForwardChecksAccessOnBothSides(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	mariID, mariTok := e.createUser("mari", "Mari")
	e.addMember(general, mariID, true)
	secret := e.createChannel("secret")
	hidden, _ := e.send(server.DevAdminToken, secret, map[string]any{"body": "secret plan"})
	open, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "public note"})

	// A source Mari cannot read is not found.
	if s, _ := e.forward(mariTok, hidden.ID, map[string]any{"channel_id": general}); s != http.StatusNotFound {
		t.Fatalf("forward from an unreadable channel: %d", s)
	}
	// A target Mari cannot write to is refused.
	readonly := e.createChannel("announcements")
	e.addMember(readonly, mariID, false)
	if s, _ := e.forward(mariTok, open.ID, map[string]any{"channel_id": readonly}); s != http.StatusForbidden {
		t.Fatalf("forward to a read-only channel: %d", s)
	}
	if s, _ := e.forward(mariTok, open.ID, map[string]any{"channel_id": secret}); s != http.StatusForbidden && s != http.StatusNotFound {
		t.Fatalf("forward to a channel she is not in: %d", s)
	}
	if s, _ := e.forward(server.DevAgentToken, open.ID, map[string]any{"channel_id": general}); s != http.StatusForbidden {
		t.Fatalf("agent forward: %d", s)
	}

	// Out of a private channel into one Mari reads: she sees the quote,
	// but not the mark naming a message she cannot open.
	s, res := e.forward(server.DevAdminToken, hidden.ID, map[string]any{"channel_id": general})
	if s != http.StatusCreated {
		t.Fatalf("forward out of the private channel: %d", s)
	}
	for _, m := range func() []apispec.Message {
		var out apispec.MessageList
		e.do("GET", "/api/v1/channels/"+general+"/messages", mariTok, nil, &out)
		return out.Messages
	}() {
		if m.Id == res.Message.Id && m.ForwardedFromId != nil {
			t.Fatal("the forward mark names a message the reader cannot open")
		}
	}

	gone, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "oops"})
	e.do("POST", "/api/v1/messages/"+gone.ID+"/retract", server.DevAdminToken, nil, nil)
	if s, _ := e.forward(server.DevAdminToken, gone.ID, map[string]any{"channel_id": general}); s != http.StatusBadRequest {
		t.Fatalf("forward a retracted message: %d", s)
	}
}
