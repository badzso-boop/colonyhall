// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func (e *env) pins(token, channelID string) []apispec.PinnedMessage {
	e.t.Helper()
	var out apispec.PinList
	if s := e.do("GET", "/api/v1/channels/"+channelID+"/pins", token, nil, &out); s != http.StatusOK {
		e.t.Fatalf("list pins: %d", s)
	}
	return out.Pins
}

func (e *env) pin(token, messageID string) int {
	e.t.Helper()
	return e.do("PUT", "/api/v1/messages/"+messageID+"/pin", token, nil, nil)
}

// Round 55, part D: members who can write pin and unpin; readers see the list.
func TestPinsByPeople(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	readerID, readerTok := e.createUser("olvaso", "Olvasó")
	e.addMember(general, readerID, false)
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "the decision: postgres"})

	if s := e.pin(readerTok, m.ID); s != http.StatusForbidden {
		t.Fatalf("a read-only member pinned: %d", s)
	}
	if s := e.pin(server.DevAdminToken, m.ID); s != http.StatusNoContent {
		t.Fatalf("pin: %d", s)
	}
	if s := e.pin(server.DevAdminToken, m.ID); s != http.StatusNoContent {
		t.Fatalf("pinning again is a no-op: %d", s)
	}
	pins := e.pins(readerTok, general)
	if len(pins) != 1 || pins[0].Message.Id.String() != m.ID || pins[0].Message.Pinned == nil || !*pins[0].Message.Pinned {
		t.Fatalf("pins as the reader sees them: %+v", pins)
	}
	pinned := false
	for _, msg := range e.getMessages(general) {
		if msg.Id.String() == m.ID && msg.Pinned != nil && *msg.Pinned {
			pinned = true
		}
	}
	if !pinned {
		t.Fatal("the message list does not mark the pin")
	}
	if s := e.do("DELETE", "/api/v1/messages/"+m.ID+"/pin", readerTok, nil, nil); s != http.StatusForbidden {
		t.Fatalf("a read-only member unpinned: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/messages/"+m.ID+"/pin", server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("unpin: %d", s)
	}
	if got := e.pins(server.DevAdminToken, general); len(got) != 0 {
		t.Fatalf("after unpin: %+v", got)
	}

	// A private channel's pins are not found for outsiders.
	secret := e.createChannel("secret")
	sm, _ := e.send(server.DevAdminToken, secret, map[string]any{"body": "hidden"})
	if s := e.pin(readerTok, sm.ID); s != http.StatusNotFound {
		t.Fatalf("outsider pin: %d", s)
	}
	if s := e.do("GET", "/api/v1/channels/"+secret+"/pins", readerTok, nil, nil); s != http.StatusNotFound {
		t.Fatalf("outsider list: %d", s)
	}

	// A retracted message cannot be pinned.
	gone, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "oops"})
	if s := e.do("POST", "/api/v1/messages/"+gone.ID+"/retract", server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("retract: %d", s)
	}
	if s := e.pin(server.DevAdminToken, gone.ID); s != http.StatusBadRequest {
		t.Fatalf("pin a retracted message: %d", s)
	}
}

// At most 50 pins per conversation.
func TestPinLimitPerConversation(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	for i := 0; i < 50; i++ {
		m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": fmt.Sprintf("note %d", i)})
		if s := e.pin(server.DevAdminToken, m.ID); s != http.StatusNoContent {
			t.Fatalf("pin %d: %d", i, s)
		}
	}
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "one too many"})
	if s := e.pin(server.DevAdminToken, m.ID); s != http.StatusConflict {
		t.Fatalf("51st pin: %d", s)
	}
}

// Agents pin only their own messages, at most 5 per conversation a day,
// only where they can write, and never unpin.
func TestAgentPins(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	human, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "a person's message"})
	if out, isErr := e.mcpCall(server.DevAgentToken, "pin", map[string]any{"message": human.ID}); !isErr {
		t.Fatalf("an agent pinned someone else's message: %s", out)
	}
	if s := e.pin(server.DevAgentToken, human.ID); s != http.StatusForbidden {
		t.Fatalf("REST: an agent pinned someone else's message: %d", s)
	}

	var own []string
	for i := 0; i < 6; i++ {
		m, s := e.send(server.DevAgentToken, general, map[string]any{"body": fmt.Sprintf("result number %d is ready", i)})
		if s != http.StatusCreated {
			t.Fatalf("agent send %d: %d", i, s)
		}
		own = append(own, m.ID)
	}
	for i := 0; i < 5; i++ {
		if out, isErr := e.mcpCall(server.DevAgentToken, "pin", map[string]any{"message": own[i]}); isErr {
			t.Fatalf("agent pin %d: %s", i, out)
		}
	}
	if out, isErr := e.mcpCall(server.DevAgentToken, "pin", map[string]any{"message": own[5]}); !isErr {
		t.Fatalf("the 6th pin in a day went through: %s", out)
	}
	// Unpinning does not give the agent its quota back, and agents do not unpin.
	if s := e.do("DELETE", "/api/v1/messages/"+own[0]+"/pin", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent unpin: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/messages/"+own[0]+"/pin", server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("human unpin: %d", s)
	}
	if s := e.pin(server.DevAgentToken, own[5]); s != http.StatusTooManyRequests {
		t.Fatalf("after an unpin the agent still has no quota: %d", s)
	}
	// A day later the agent may pin again.
	e.clock.Advance(25 * time.Hour)
	if s := e.pin(server.DevAgentToken, own[5]); s != http.StatusNoContent {
		t.Fatalf("pin after a day: %d", s)
	}

	// Where it cannot write, an agent cannot pin either (its own old message).
	agentID := e.agentID()
	e.setCanWrite(general, agentID, false)
	if s := e.pin(server.DevAgentToken, own[1]); s != http.StatusForbidden {
		t.Fatalf("pin while read-only: %d", s)
	}
	if out, isErr := e.mcpCall(server.DevAgentToken, "pin", map[string]any{"message": own[4]}); !isErr {
		t.Fatalf("a read-only agent pinned: %s", out)
	}
}

// send important: true pins the message, marks it, and counts toward the limit.
func TestAgentImportantSend(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	out, isErr := e.mcpCall(server.DevAgentToken, "send", map[string]any{"channel": "general", "text": "deploy finished, 3 tests need a look", "important": true})
	if isErr || !strings.Contains(out, "pinned") {
		t.Fatalf("important send: %s", out)
	}
	pins := e.pins(server.DevAdminToken, general)
	if len(pins) != 1 || pins[0].Message.Important == nil || !*pins[0].Message.Important {
		t.Fatalf("important message not pinned and marked: %+v", pins)
	}
	// People pin instead of marking.
	if _, s := e.send(server.DevAdminToken, general, map[string]any{"body": "x", "important": true}); s != http.StatusBadRequest {
		t.Fatalf("a person sent important: %d", s)
	}
	for i := 0; i < 4; i++ {
		if out, isErr := e.mcpCall(server.DevAgentToken, "send", map[string]any{"channel": "general", "text": fmt.Sprintf("result %d", i), "important": true}); isErr {
			t.Fatalf("important %d: %s", i, out)
		}
	}
	before := len(e.getMessages(general))
	if out, isErr := e.mcpCall(server.DevAgentToken, "send", map[string]any{"channel": "general", "text": "one more result", "important": true}); !isErr {
		t.Fatalf("the 6th important message went through: %s", out)
	}
	if after := len(e.getMessages(general)); after != before {
		t.Fatalf("a refused important send still posted (%d → %d messages)", before, after)
	}
}
