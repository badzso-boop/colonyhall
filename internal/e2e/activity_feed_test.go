// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func (e *env) activity(token, query string) apispec.ActivityPage {
	e.t.Helper()
	if err := e.app.Drain(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	var out apispec.ActivityPage
	if s := e.do("GET", "/api/v1/me/activity"+query, token, nil, &out); s != http.StatusOK {
		e.t.Fatalf("activity: %d", s)
	}
	return out
}

func kinds(p apispec.ActivityPage) []string {
	out := make([]string, len(p.Items))
	for i, it := range p.Items {
		out[i] = string(it.Kind)
	}
	return out
}

// Round 55, part H: mentions, replies in my threads, reactions to my
// messages, important messages and due reminders, newest first, only mine.
func TestActivityFeed(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	mariID, mariTok := e.createUser("mari", "Mari")
	e.addMember(general, mariID, true)

	root, _ := e.send(mariTok, general, map[string]any{"body": "a question about the release"})
	e.send(server.DevAdminToken, general, map[string]any{"body": "@mari can you look?"})
	e.clock.Advance(time.Second)
	e.send(server.DevAdminToken, general, map[string]any{"body": "answer in the thread", "thread_root_id": root.ID})
	e.clock.Advance(time.Second)
	if s := e.do("PUT", reactionPath(root.ID, "👍"), server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("react: %d", s)
	}
	e.clock.Advance(time.Second)
	if out, isErr := e.mcpCall(server.DevAgentToken, "send", map[string]any{"channel": "general", "text": "the release is out", "important": true}); isErr {
		t.Fatalf("important: %s", out)
	}

	page := e.activity(mariTok, "")
	got := kinds(page)
	want := []string{"important", "reaction", "thread_reply", "mention"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("feed: %v, want %v", got, want)
	}
	if page.UnreadCount != 4 {
		t.Fatalf("unread: %d", page.UnreadCount)
	}
	mention := page.Items[3]
	if !mention.Available || mention.Excerpt == nil || !strings.Contains(*mention.Excerpt, "@mari") || mention.ChannelName == nil || *mention.ChannelName != "general" {
		t.Fatalf("mention item: %+v", mention)
	}
	if page.Items[1].Emoji == nil || *page.Items[1].Emoji != "👍" {
		t.Fatalf("reaction item: %+v", page.Items[1])
	}
	// The author's own feed has none of their own doing.
	for _, it := range e.activity(server.DevAdminToken, "").Items {
		if it.Kind == apispec.ActivityKindMention || it.Kind == apispec.ActivityKindReaction {
			t.Fatalf("own action in own feed: %+v", it)
		}
	}
	if s := e.do("GET", "/api/v1/me/activity", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent: %d", s)
	}

	// Filters, then marking read: one item, then the rest.
	if f := e.activity(mariTok, "?kind=mention"); len(f.Items) != 1 {
		t.Fatalf("kind filter: %v", kinds(f))
	}
	var marked apispec.MarkedCount
	e.do("POST", "/api/v1/me/activity/read", mariTok, map[string]any{"item_ids": []string{mention.Id.String()}}, &marked)
	if marked.Marked != 1 || e.activity(mariTok, "").UnreadCount != 3 {
		t.Fatalf("mark one: %+v", marked)
	}
	if u := e.activity(mariTok, "?unread=true"); len(u.Items) != 3 {
		t.Fatalf("unread filter: %v", kinds(u))
	}
	e.do("POST", "/api/v1/me/activity/read", mariTok, nil, &marked)
	if marked.Marked != 3 || e.activity(mariTok, "").UnreadCount != 0 {
		t.Fatalf("mark all: %+v", marked)
	}
}

// A conversation set to "none" adds nothing but what waits for the person;
// a conversation they lost shows nothing of itself.
func TestActivityFollowsLevelAndAccess(t *testing.T) {
	e := newEnv(t)
	mariID, mariTok := e.createUser("mari", "Mari")
	ops := e.createChannel("ops", mariID)
	if s := e.do("PUT", "/api/v1/channels/"+ops+"/notify", mariTok, map[string]any{"level": "none"}, nil); s != http.StatusNoContent {
		t.Fatalf("mute: %d", s)
	}
	e.send(server.DevAdminToken, ops, map[string]any{"body": "@mari muted mention"})
	if got := e.activity(mariTok, ""); len(got.Items) != 0 {
		t.Fatalf("a muted conversation's mention: %v", kinds(got))
	}
	e.do("DELETE", "/api/v1/channels/"+ops+"/mute", mariTok, nil, nil)
	e.send(server.DevAdminToken, ops, map[string]any{"body": "@mari ops secret"})
	if got := e.activity(mariTok, ""); len(got.Items) != 1 {
		t.Fatalf("mention after unmute: %v", kinds(got))
	}
	if s := e.do("DELETE", "/api/v1/channels/"+ops+"/members/"+mariID, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("remove: %d", s)
	}
	it := e.activity(mariTok, "").Items[0]
	if it.Available || it.Excerpt != nil || it.ChannelName != nil || it.MessageId != nil || it.ActorId != nil || it.RefId != nil {
		t.Fatalf("a lost conversation leaks through the feed: %+v", it)
	}
}

// Questions to a person and due reminders are in the feed.
func TestActivityQuestionsAndReminders(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	if out, isErr := e.mcpCall(server.DevAgentToken, "ask", map[string]any{"question": "deploy now?", "to": "@norbi", "channel": "#general"}); isErr {
		t.Fatalf("ask: %s", out)
	}
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "invoice"})
	e.save(server.DevAdminToken, map[string]any{"message_id": m.ID, "remind_at": e.clock.Now().Add(time.Minute)})
	e.clock.Advance(2 * time.Minute)
	e.runDueReminders()
	got := e.activity(server.DevAdminToken, "")
	have := map[string]bool{}
	for _, it := range got.Items {
		have[string(it.Kind)] = true
		if it.Kind == apispec.ActivityKindReminder && (it.Excerpt == nil || !strings.Contains(*it.Excerpt, "⏰")) {
			t.Fatalf("reminder item: %+v", it)
		}
	}
	if !have["question"] || !have["reminder"] {
		t.Fatalf("feed: %v", kinds(got))
	}
}

// The badge signal reaches its recipient alone.
func TestActivityEventsArePersonal(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	mariID, mariTok := e.createUser("mari", "Mari")
	e.addMember(general, mariID, true)
	_, bossTok := e.createActor("human", "boss", "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	other := e.dialWS(ctx, bossTok)
	mine := e.dialWS(ctx, mariTok)
	e.send(server.DevAdminToken, general, map[string]any{"body": "@mari hi"})
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.send(server.DevAdminToken, general, map[string]any{"body": "marker"})
	for _, f := range eventsUntilMessage(t, ctx, other, "marker") {
		if strings.HasPrefix(f.EventType, "activity.") {
			t.Fatal("another admin got the activity event")
		}
	}
	seen := false
	for _, f := range eventsUntilMessage(t, ctx, mine, "marker") {
		if f.EventType == "activity.created" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("the recipient got no activity event")
	}
}
