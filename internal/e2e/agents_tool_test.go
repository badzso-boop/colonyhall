// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"colonyhall/internal/apispec"
	"colonyhall/internal/fakeagent"
	"colonyhall/internal/server"
)

// The agents tool: an agent finds the other Colonyhall agents (its client's
// own agent tools cannot see them), where they run and how to give them a
// task, without folders or other configuration (found 2026-11-10: an agent
// answered that an existing agent did not exist).
func TestAgentsTool(t *testing.T) {
	f := newOwnerFixture(t)
	e := f.e
	if s := e.do("PATCH", "/api/v1/actors/"+f.otherSandbox+"/profile", server.DevAdminToken,
		map[string]any{"description": "Reads the trader logs and   summarises\nthe runs."}, nil); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("profile: %d", s)
	}
	gone, _ := e.newAgent("old-agent", "Old", nil, nil)
	if s := e.do("DELETE", "/api/v1/agents/"+gone, server.DevAdminToken, nil, nil); s != http.StatusNoContent && s != http.StatusOK {
		t.Fatalf("archive: %d", s)
	}

	out, isErr := e.mcpCall(f.otherSandTok, "agents", nil)
	if isErr {
		t.Fatalf("agents: %s", out)
	}
	line := func(handle string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(l, "- @"+handle+" ") {
				return l
			}
		}
		t.Fatalf("@%s missing:\n%s", handle, out)
		return ""
	}
	if l := line("norbi-full"); !strings.Contains(l, "full access, claude-code on homelab (online)") ||
		!strings.Contains(l, "waits for a human admin's approval") {
		t.Fatalf("full-access agent: %s", l)
	}
	if l := line("norbi-sand"); !strings.Contains(l, "— you") || !strings.Contains(l, "sandboxed") ||
		!strings.Contains(l, "an agent may give it a task directly") {
		t.Fatalf("own line: %s", l)
	}
	if !strings.Contains(out, "\n  Reads the trader logs and summarises the runs.\n") {
		t.Fatalf("description not on one line:\n%s", out)
	}
	if l := line("kollega-coder"); !strings.Contains(l, "owner @kollega") {
		t.Fatalf("owner: %s", l)
	}
	if strings.Contains(out, "old-agent") {
		t.Fatalf("an archived agent is listed:\n%s", out)
	}
	if strings.Contains(out, "/app") {
		t.Fatalf("a folder leaked:\n%s", out)
	}
	if !strings.Contains(out, "People: ") || !strings.Contains(out, "@norbi (admin)") || !strings.Contains(out, "@kollega") {
		t.Fatalf("people:\n%s", out)
	}

	out, _ = e.mcpCall(f.otherSandTok, "agents", map[string]any{"query": "PLANNER"})
	if !strings.Contains(out, "@kollega-planner") || strings.Contains(out, "@norbi-full") {
		t.Fatalf("query:\n%s", out)
	}
}

// A request another agent sends from a thread carries that thread to the
// recipient (request_origin) when the recipient may write there, so the
// recipient answers in the thread, linked to the request; otherwise it stays
// a plain DM (found 2026-11-10: the answer stayed in a DM, and the thread's
// session knew nothing about it).
func TestRequestOriginFromThread(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	general := e.generalID()
	askerID, askerTok := e.createActor("agent", "asker", "member")
	helperID, helperTok := e.createActor("agent", "helper", "member")
	_, outsiderTok := e.createActor("agent", "outsider", "member")
	for _, id := range []string{askerID, helperID} {
		e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": id, "can_write": true}, nil)
	}
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@asker please get this implemented"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	asker, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", askerTok, headerTripper{
		headers: map[string]string{"X-Colonyhall-Channel": general, "X-Colonyhall-Thread": root.ID}, base: http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = asker.Close() }()
	for _, to := range []string{"@helper", "@outsider"} {
		if res, err := asker.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": to, "text": "Please implement the Umami component in the layout."}}); err != nil || res.IsError {
			t.Fatalf("request to %s: %v %+v", to, err, res)
		}
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	request := func(tok string) apispec.InboxItem {
		var ib apispec.Inbox
		e.do("GET", "/api/v1/agent/inbox", tok, nil, &ib)
		for _, it := range ib.Items {
			if it.Author == "asker" {
				return it
			}
		}
		t.Fatalf("the request did not arrive: %+v", ib.Items)
		return apispec.InboxItem{}
	}
	it := request(helperTok)
	if it.RequestOrigin == nil || it.RequestOrigin.ChannelId.String() != general || it.RequestOrigin.ThreadRootId.String() != root.ID ||
		it.RequestOrigin.RequestMessageId.String() != it.ItemId.String() || it.ReplyOrigin != nil {
		t.Fatalf("request_origin: %+v", it.RequestOrigin)
	}
	// Not a member of the thread's channel: a plain DM, nothing about the thread.
	if o := request(outsiderTok); o.RequestOrigin != nil {
		t.Fatalf("the outsider learned about the thread: %+v", o.RequestOrigin)
	}

	// A requester that may only read the thread's channel cannot get its
	// request answered there through the helper.
	readerID, readerTok := e.createActor("agent", "reader", "member")
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": readerID, "can_write": false}, nil)
	reader, err := fakeagent.Connect(ctx, e.srv.URL+"/mcp", readerTok, headerTripper{
		headers: map[string]string{"X-Colonyhall-Channel": general, "X-Colonyhall-Thread": root.ID}, base: http.DefaultTransport,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	if res, err := reader.CallTool(ctx, &mcp.CallToolParams{Name: "send", Arguments: map[string]any{"channel": "@helper", "text": "Please post the release notes."}}); err != nil || res.IsError {
		t.Fatalf("reader request: %v %+v", err, res)
	}
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var ib apispec.Inbox
	e.do("GET", "/api/v1/agent/inbox", helperTok, nil, &ib)
	for _, x := range ib.Items {
		if x.Author == "reader" && x.RequestOrigin != nil {
			t.Fatalf("a read-only requester got the answer routed into the channel: %+v", x.RequestOrigin)
		}
	}

	// The helper answers in the thread, linked to the request.
	var r sendResult
	s := e.do("POST", "/api/v1/channels/"+general+"/messages", helperTok, map[string]any{
		"body": "Done: UmamiAnalytics.tsx is in the layout.", "thread_root_id": root.ID, "answers_message_id": it.ItemId.String(),
	}, &r)
	if s != http.StatusCreated || r.Message == nil {
		t.Fatalf("answer: %d", s)
	}
	var thread apispec.Thread
	e.do("GET", "/api/v1/messages/"+root.ID+"/thread", server.DevAdminToken, nil, &thread)
	var found bool
	for _, m := range thread.Replies {
		if m.Body == "Done: UmamiAnalytics.tsx is in the layout." {
			found = m.Answers != nil && m.Answers.MessageId.String() == it.ItemId.String()
		}
	}
	if !found {
		t.Fatalf("the answer in the thread is not linked to the request: %+v", thread.Replies)
	}
}
