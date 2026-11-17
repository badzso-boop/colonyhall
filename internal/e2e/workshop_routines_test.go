// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/modules/workshop"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/server"
)

const jiraRoutineSource = `
export async function poll(input: any, ctx: any) {
  const p = input.trigger.payload ?? {};
  return { text: "Új számla: " + (p.invoice ?? "?"), blocks: [
    { type: "text", text: "Új számla: " + (p.invoice ?? "?") + ". Rendben?" },
    { type: "buttons", buttons: [{ action_id: "ok", label: "Rendben", style: "primary" }, { action_id: "ask", label: "Kérdezek" }] },
  ] };
}
export async function thanks(input: any, ctx: any) {
  return "Köszönöm, " + input.trigger.actor + ": " + input.trigger.label;
}
`

func jiraRoutineManifest() map[string]any {
	return map[string]any{
		"connectors": []string{"jira"},
		"tools": []map[string]any{
			{"name": "mine", "kind": "http", "risk": "read", "request": map[string]any{"connector": "jira", "method": "GET", "path": "/rest/search",
				"query": map[string]any{"jql": "assignee = currentUser()"}},
				"output": map[string]any{"pick": map[string]any{"key": "issues[].key", "summary": "issues[].fields.summary"}}},
			{"name": "poll", "kind": "js", "risk": "read"},
			{"name": "thanks", "kind": "js", "risk": "read"},
		},
		"routines": []map[string]any{
			{"name": "daily", "trigger": map[string]any{"cron": "0 8 * * 1-5"}, "tool": "mine", "output": "napi-feladatok"},
			{"name": "standup", "trigger": map[string]any{"cron": "30 8 * * 1-5"}, "agent": "helper", "prompt": "Foglald össze a mai Jira-feladatokat.", "output": "napi-feladatok"},
			{"name": "invoice", "trigger": map[string]any{"webhook": true}, "tool": "poll", "output": "napi-feladatok"},
			{"name": "asked", "trigger": map[string]any{"message": map[string]any{"channel": "napi-feladatok", "contains": "jira?"}}, "tool": "mine"},
			{"name": "clicked", "trigger": map[string]any{"action": "ok"}, "tool": "thanks"},
		},
	}
}

type chMsg struct {
	ID           string `json:"id"`
	Body         string `json:"body"`
	AuthorID     string `json:"author_id"`
	ThreadRootID string `json:"thread_root_id"`
	Blocks       []any  `json:"blocks"`
}

func (e *env) messagesIn(channelID string) []chMsg {
	var r struct {
		Messages []chMsg `json:"messages"`
	}
	e.do("GET", "/api/v1/channels/"+channelID+"/messages?limit=50", server.DevAdminToken, nil, &r)
	return r.Messages
}

func (e *env) threadOf(rootID string) []chMsg {
	var r struct {
		Replies []chMsg `json:"replies"`
	}
	e.do("GET", "/api/v1/messages/"+rootID+"/thread", server.DevAdminToken, nil, &r)
	return r.Replies
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// work runs the routine loop's two steps once and waits for the runs.
func work(t *testing.T, e *env) {
	t.Helper()
	ctx := system.Context(context.Background())
	if _, err := e.app.Workshop.TickRoutines(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{}, 50)
	n, err := e.app.Workshop.WorkRoutines(ctx, func() { done <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("a routine run did not finish")
		}
	}
}

// TestJiraRoutinesEndToEnd is docs/25's example: every weekday at 8 the
// extension's bot posts the Jira issues to #napi-feladatok, at 8:30 it asks
// an agent for a summary; a webhook posts an interactive message whose
// button runs a routine; a question in the channel gets an answer in its
// thread.
func TestJiraRoutinesEndToEnd(t *testing.T) {
	fetch := &fakeFetcher{}
	e := extEnv(t, fetch, workshop.Limits{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = e.app.Run(ctx) }()
	admin := server.DevAdminToken

	var ch struct {
		ID string `json:"id"`
	}
	if s := e.do("POST", "/api/v1/channels", admin, map[string]any{"name": "napi-feladatok"}, &ch); s != http.StatusCreated {
		t.Fatalf("channel: %d", s)
	}
	agentID, agentTok := e.newAgent("helper", "Helper", nil, nil)
	e.do("POST", "/api/v1/channels/"+ch.ID+"/members", admin, map[string]any{"actor_ids": []string{agentID}}, nil)
	if s := e.do("POST", "/api/v1/workshop/connectors", admin, map[string]any{"slug": "jira", "title": "Jira", "kind": "http",
		"base_url": "https://jira.example.com", "auth": "bearer", "secret": testExtSecret}, nil); s != http.StatusCreated {
		t.Fatalf("connector: %d", s)
	}
	var ext apispec.Extension
	e.do("POST", "/api/v1/workshop/extensions", admin, map[string]any{"slug": "jira", "title": "Napi Jira", "bot_handle": "jira-bot"}, &ext)
	id := ext.Id.String()
	var v apispec.ExtensionVersion
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions", admin, map[string]any{"manifest": jiraRoutineManifest(), "source": jiraRoutineSource}, &v); s != http.StatusCreated {
		t.Fatalf("version: %d", s)
	}
	if v.Diff.NewRoutines == nil || len(*v.Diff.NewRoutines) != 5 {
		t.Fatalf("the diff lists the routines: %+v", v.Diff)
	}
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/approve", admin, nil, &ext); s != http.StatusOK {
		t.Fatalf("approve: %d", s)
	}
	var rl apispec.RoutineList
	e.do("GET", "/api/v1/workshop/extensions/"+id+"/routines", admin, nil, &rl)
	if len(rl.Routines) != 5 || rl.Routines[0].NextRunAt == nil {
		t.Fatalf("routines: %+v", rl.Routines)
	}

	// The next weekday morning: both cron routines run.
	e.clock.Advance(24 * time.Hour)
	work(t, e)
	msgs := e.messagesIn(ch.ID)
	var daily, standup *chMsg
	for i, m := range msgs {
		if m.AuthorID == ext.BotId.String() && strings.Contains(m.Body, "PROJ-1") {
			daily = &msgs[i]
		}
		if m.AuthorID == ext.BotId.String() && strings.HasPrefix(m.Body, "@helper Foglald") {
			standup = &msgs[i]
		}
	}
	if daily == nil || standup == nil {
		t.Fatalf("the morning posts: %+v", msgs)
	}
	if fetch.last().Auth["Authorization"] != "Bearer "+testExtSecret {
		t.Fatal("the cron routine's request had no secret")
	}
	// The agent was asked.
	waitFor(t, "the agent's inbox", func() bool {
		out, _ := e.mcpCall(agentTok, "inbox", nil)
		return strings.Contains(out, "Foglald össze")
	})

	// A webhook posts an interactive message.
	var hook apispec.RoutineWebhook
	if s := e.do("POST", "/api/v1/workshop/extensions/"+id+"/routines/invoice/webhook", admin, nil, &hook); s != http.StatusOK {
		t.Fatalf("webhook address: %d", s)
	}
	post := func(path, body string) int {
		res, err := http.Post(e.srv.URL+path, "application/json", bytes.NewReader([]byte(body)))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		return res.StatusCode
	}
	if s := post(hook.Path+"x", `{}`); s != http.StatusNotFound {
		t.Fatalf("a wrong token: %d", s)
	}
	if s := post(hook.Path, `{"invoice":"2026/42"}`); s != http.StatusAccepted {
		t.Fatalf("webhook: %d", s)
	}
	work(t, e)
	var poll *chMsg
	for _, m := range e.messagesIn(ch.ID) {
		if len(m.Blocks) == 2 && strings.Contains(m.Body, "2026/42") {
			poll = &m
		}
	}
	if poll == nil {
		t.Fatalf("the interactive message: %+v", e.messagesIn(ch.ID))
	}

	// A person clicks: the action routine answers in the thread.
	var ar apispec.MessageActionResult
	if s := e.do("POST", "/api/v1/messages/"+poll.ID+"/actions", admin, map[string]any{"action_id": "nope"}, nil); s != http.StatusBadRequest {
		t.Fatalf("an unknown action: %d", s)
	}
	if s := e.do("POST", "/api/v1/messages/"+poll.ID+"/actions", admin, map[string]any{"action_id": "ok"}, &ar); s != http.StatusOK || ar.Label != "Rendben" {
		t.Fatalf("click: %d %+v", s, ar)
	}
	waitFor(t, "the action routine", func() bool {
		work(t, e)
		for _, m := range e.threadOf(poll.ID) {
			if strings.Contains(m.Body, "Köszönöm, @norbi: Rendben") {
				return true
			}
		}
		return false
	})

	// A question in the channel: the answer in its thread.
	q, _ := e.send(admin, ch.ID, map[string]any{"body": "Mi a helyzet a jira? feladatokkal"})
	waitFor(t, "the message routine", func() bool {
		work(t, e)
		for _, m := range e.threadOf(q.ID) {
			if m.AuthorID == ext.BotId.String() && strings.Contains(m.Body, "PROJ-2") {
				return true
			}
		}
		return false
	})

	e.do("GET", "/api/v1/workshop/extensions/"+id+"/routines", admin, nil, &rl)
	if len(rl.Runs) < 5 {
		t.Fatalf("runs: %+v", rl.Runs)
	}
	for _, r := range rl.Runs {
		if r.State != "done" {
			raw, _ := json.Marshal(r)
			t.Fatalf("a run did not finish well: %s", raw)
		}
	}
}

// A click on an agent's interactive message reaches the agent as a
// thread reply mentioning it.
func TestClickReachesTheAgent(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = e.app.Run(ctx) }()
	_, agentTok := e.newAgent("helper", "Helper", nil, nil)
	general := e.generalID()
	out, isErr := e.mcpCall(agentTok, "send", map[string]any{"channel": "general", "text": "Melyik legyen?", "elaborate": true,
		"blocks": []map[string]any{{"type": "select", "action_id": "pick", "options": []map[string]any{{"value": "a", "label": "Az A"}, {"value": "b", "label": "A B"}}}}})
	if isErr {
		t.Fatalf("send with blocks: %q", out)
	}
	var m *chMsg
	for _, x := range e.messagesIn(general) {
		if len(x.Blocks) == 1 {
			m = &x
		}
	}
	if m == nil {
		t.Fatal("the interactive message is not in #general")
	}
	if s := e.do("POST", "/api/v1/messages/"+m.ID+"/actions", server.DevAdminToken, map[string]any{"action_id": "pick", "value": "zzz"}, nil); s != http.StatusBadRequest {
		t.Fatalf("an unknown option: %d", s)
	}
	if s := e.do("POST", "/api/v1/messages/"+m.ID+"/actions", server.DevAdminToken, map[string]any{"action_id": "pick", "value": "b"}, nil); s != http.StatusOK {
		t.Fatalf("choose: %d", s)
	}
	waitFor(t, "the reply in the thread", func() bool {
		for _, x := range e.threadOf(m.ID) {
			if strings.Contains(x.Body, "@helper ▶ A B") {
				return true
			}
		}
		return false
	})
	waitFor(t, "the agent's inbox", func() bool {
		out, _ := e.mcpCall(agentTok, "inbox", nil)
		return strings.Contains(out, "A B")
	})
}

// Security review of round 56c: a webhook address dies with its routine
// and with a switched-off extension; a click needs the right to write.
func TestRoutineWebhookRevokedAndClickNeedsWrite(t *testing.T) {
	e := extEnv(t, &fakeFetcher{}, workshop.Limits{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = e.app.Run(ctx) }()
	admin := server.DevAdminToken
	var ch struct {
		ID string `json:"id"`
	}
	e.do("POST", "/api/v1/channels", admin, map[string]any{"name": "napi-feladatok"}, &ch)
	e.do("POST", "/api/v1/workshop/connectors", admin, map[string]any{"slug": "jira", "title": "Jira", "kind": "http",
		"base_url": "https://jira.example.com", "auth": "bearer", "secret": testExtSecret}, nil)
	var ext apispec.Extension
	e.do("POST", "/api/v1/workshop/extensions", admin, map[string]any{"slug": "jira", "title": "Napi Jira"}, &ext)
	id := ext.Id.String()
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions", admin, map[string]any{"manifest": jiraRoutineManifest(), "source": jiraRoutineSource}, nil)
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/1/approve", admin, nil, nil)
	var hook apispec.RoutineWebhook
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/routines/invoice/webhook", admin, nil, &hook)
	post := func() int {
		res, err := http.Post(e.srv.URL+hook.Path, "application/json", strings.NewReader(`{"invoice":"1"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	if s := post(); s != http.StatusAccepted {
		t.Fatalf("webhook: %d", s)
	}
	work(t, e)
	// A read-only member may see the interactive message, not click it.
	var poll *chMsg
	for _, m := range e.messagesIn(ch.ID) {
		if len(m.Blocks) > 0 {
			poll = &m
		}
	}
	if poll == nil {
		t.Fatal("no interactive message")
	}
	readerID, readerTok := e.createUser("olvaso", "Olvasó")
	e.do("POST", "/api/v1/channels/"+ch.ID+"/members", admin, map[string]any{"actor_ids": []string{readerID}, "can_write": false}, nil)
	if s := e.do("POST", "/api/v1/messages/"+poll.ID+"/actions", readerTok, map[string]any{"action_id": "ok"}, nil); s != http.StatusForbidden {
		t.Fatalf("a read-only member clicked: %d", s)
	}

	// A version without the webhook routine revokes its address.
	m := jiraRoutineManifest()
	rs := m["routines"].([]map[string]any)
	m["routines"] = []map[string]any{rs[0]}
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions", admin, map[string]any{"manifest": m, "source": jiraRoutineSource}, nil)
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/versions/2/approve", admin, nil, nil)
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/rollback", admin, map[string]any{"version": 1}, nil)
	if s := post(); s != http.StatusNotFound {
		t.Fatalf("the old address after the routine was removed: %d", s)
	}
	// A new address works until the extension is switched off.
	e.do("POST", "/api/v1/workshop/extensions/"+id+"/routines/invoice/webhook", admin, nil, &hook)
	if s := post(); s != http.StatusAccepted {
		t.Fatalf("new address: %d", s)
	}
	e.do("PUT", "/api/v1/workshop/extensions/"+id+"/enabled", admin, map[string]any{"enabled": false}, nil)
	e.do("PUT", "/api/v1/workshop/extensions/"+id+"/enabled", admin, map[string]any{"enabled": true}, nil)
	if s := post(); s != http.StatusNotFound {
		t.Fatalf("the address after switching off: %d", s)
	}
}
