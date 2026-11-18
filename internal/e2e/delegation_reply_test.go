// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

// A full-access agent asks a sandboxed one in their DM: the answer is not a
// task, so it reaches the asker without an approval question (2026-11-01).
// A message the full-access agent did not ask for still waits for approval.
func TestDelegationReplyToAskerNeedsNoApproval(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	hostID, _ := e.enroll("box")
	fullID, fullTok := createFullAccessAgent(e, hostID, "personal", "Personal")
	sbID, sbTok := createSandboxedAgent(e, hostID, "trader", "Trader")
	var dm apispec.Channel
	if s := e.do("POST", "/api/v1/dms", server.DevAdminToken, map[string]any{"actor_ids": []string{fullID, sbID}}, &dm); s != http.StatusOK && s != http.StatusCreated {
		t.Fatalf("open DM: %d", s)
	}
	pending := func() int {
		var l apispec.DelegationList
		e.do("GET", "/api/v1/delegations?state=pending", server.DevAdminToken, nil, &l)
		return len(l.Delegations)
	}
	inbox := func(tok string) apispec.Inbox {
		var b apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", tok, nil, &b)
		return b
	}

	// The full-access agent asks; the sandboxed one may get tasks freely.
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dm.Id), fullTok, apispec.SendMessageRequest{Body: "@trader foglald össze a futásokat"}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if b := inbox(sbTok); b.Total != 1 {
		t.Fatalf("the sandboxed agent's inbox: %d", b.Total)
	}

	// Its answer goes straight back, no question, and wakes the asker.
	e.clock.Advance(5 * time.Minute)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dm.Id), sbTok, apispec.SendMessageRequest{Body: "@personal 12 futás, +3,4 %"}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if n := pending(); n != 0 {
		t.Fatalf("the answer waits for approval: %d pending", n)
	}
	b := inbox(fullTok)
	// An answer item: the guard keeps it for a full-access agent (a plain
	// agent message it refuses as a task).
	if b.Total != 1 || b.Items[0].Kind != "answer" || b.Items[0].Author != "trader" || b.Items[0].Body != "@personal 12 futás, +3,4 %" {
		t.Fatalf("the asker's inbox: %+v", b)
	}

	// Long after the question (beyond the grant idle window), a new message is a task again.
	e.clock.Advance(3 * time.Hour)
	e.do("POST", fmt.Sprintf("/api/v1/channels/%s/messages", dm.Id), sbTok, apispec.SendMessageRequest{Body: "@personal futtasd le a migrációt"}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if n := pending(); n != 1 {
		t.Fatalf("an unasked task: %d pending", n)
	}

	// In #general the full-access agent never asked: a task waits for approval.
	general := e.generalID()
	e.do("POST", "/api/v1/channels/"+general+"/messages", sbTok, apispec.SendMessageRequest{Body: "@personal nézd meg a logokat"}, nil)
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if n := pending(); n != 2 {
		t.Fatalf("a task in another channel: %d pending", n)
	}
}

// A full-access agent asks a sandboxed one in a channel thread: the answer,
// without a mention, reaches it as an answer item and needs no approval
// (live test, 2026-11-18).
func TestDelegationThreadAnswerReachesFullAccessAsker(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	hostID, _ := e.enroll("box")
	fullID, fullTok := createFullAccessAgent(e, hostID, "personal", "Personal")
	sbID, sbTok := createSandboxedAgent(e, hostID, "trader", "Trader")
	general := e.generalID()
	for _, id := range []string{fullID, sbID} {
		e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": id, "can_write": true}, nil)
	}
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@personal mi a helyzet a futásokkal?"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	ackAll := func(tok string) {
		var b apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", tok, nil, &b)
		var list []string
		for _, it := range b.Items {
			list = append(list, it.Id.String())
		}
		if len(list) > 0 {
			e.do("POST", "/api/v1/agent/inbox/ack", tok, map[string]any{"ids": list}, nil)
		}
	}
	ackAll(fullTok)
	e.send(fullTok, general, map[string]any{"body": "@trader hány futás volt ma?", "thread_root_id": root.ID})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	ackAll(sbTok)
	e.clock.Advance(2 * time.Minute)
	e.send(sbTok, general, map[string]any{"body": "Ma 12 futás volt.", "thread_root_id": root.ID})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var l apispec.DelegationList
	e.do("GET", "/api/v1/delegations?state=pending", server.DevAdminToken, nil, &l)
	if len(l.Delegations) != 0 {
		t.Fatalf("the answer waits for approval: %+v", l.Delegations)
	}
	var b apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", fullTok, nil, &b)
	if b.Total != 1 || b.Items[0].Kind != "answer" || b.Items[0].Body != "Ma 12 futás volt." {
		t.Fatalf("the asker's inbox: %+v", b)
	}
}
