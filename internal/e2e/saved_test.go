// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/db/system"
	"colonyhall/internal/server"
)

func (e *env) savedList(token, filter string) []apispec.SavedItem {
	e.t.Helper()
	var out apispec.SavedPage
	path := "/api/v1/me/saved"
	if filter != "" {
		path += "?filter=" + filter
	}
	if s := e.do("GET", path, token, nil, &out); s != http.StatusOK {
		e.t.Fatalf("list saved: %d", s)
	}
	return out.Items
}

func (e *env) save(token string, body map[string]any) (int, apispec.SavedItem) {
	e.t.Helper()
	var it apispec.SavedItem
	return e.do("POST", "/api/v1/me/saved", token, body, &it), it
}

func (e *env) runDueReminders() int {
	e.t.Helper()
	n, err := e.app.Saved.RunDue(system.Context(context.Background()))
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.app.Drain(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// selfDM finds the person's own DM (only them in it) in their channel list.
func (e *env) selfDM(token, actorID string) *apispec.Channel {
	e.t.Helper()
	var list apispec.ChannelList
	e.do("GET", "/api/v1/channels", token, nil, &list)
	for _, c := range list.Channels {
		if c.Kind == apispec.Dm && c.MemberIds != nil && len(*c.MemberIds) == 1 && (*c.MemberIds)[0].String() == actorID {
			return &c
		}
	}
	return nil
}

// Round 55, part E: saved items are personal.
func TestSavedItemsArePersonal(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	otherID, otherTok := e.createUser("mari", "Mari")
	e.addMember(general, otherID, true)
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "read this later"})

	s, it := e.save(server.DevAdminToken, map[string]any{"message_id": m.ID})
	if s != http.StatusOK || !it.Available || it.Message == nil || it.Message.Body != "read this later" || it.Message.ChannelName != "general" {
		t.Fatalf("save: %d %+v", s, it)
	}
	// Saving again is the same item.
	if _, again := e.save(server.DevAdminToken, map[string]any{"message_id": m.ID}); again.Id != it.Id {
		t.Fatalf("saved twice: %v vs %v", again.Id, it.Id)
	}
	if got := e.savedList(server.DevAdminToken, ""); len(got) != 1 {
		t.Fatalf("own list: %+v", got)
	}
	if got := e.savedList(otherTok, ""); len(got) != 0 {
		t.Fatalf("another person sees my saved items: %+v", got)
	}
	if s := e.do("PATCH", "/api/v1/me/saved/"+it.Id.String(), otherTok, map[string]any{"done": true}, nil); s != http.StatusNotFound {
		t.Fatalf("another person's patch: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/me/saved/"+it.Id.String(), otherTok, nil, nil); s != http.StatusNotFound {
		t.Fatalf("another person's delete: %d", s)
	}
	if s, _ := e.save(server.DevAgentToken, map[string]any{"message_id": m.ID}); s != http.StatusForbidden {
		t.Fatalf("agent save: %d", s)
	}
	secret := e.createChannel("secret")
	sm, _ := e.send(server.DevAdminToken, secret, map[string]any{"body": "hidden"})
	if s, _ := e.save(otherTok, map[string]any{"message_id": sm.ID}); s != http.StatusNotFound {
		t.Fatalf("save an unreadable message: %d", s)
	}

	// Done and the filters.
	var done apispec.SavedItem
	if s := e.do("PATCH", "/api/v1/me/saved/"+it.Id.String(), server.DevAdminToken, map[string]any{"done": true}, &done); s != http.StatusOK || done.DoneAt == nil {
		t.Fatalf("mark done: %d %+v", s, done)
	}
	if len(e.savedList(server.DevAdminToken, "done")) != 1 || len(e.savedList(server.DevAdminToken, "reminders")) != 0 {
		t.Fatal("filters")
	}
	if s := e.do("DELETE", "/api/v1/me/saved/"+it.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete: %d", s)
	}
	if got := e.savedList(server.DevAdminToken, ""); len(got) != 0 {
		t.Fatalf("after delete: %+v", got)
	}
}

// A saved message the person lost access to shows nothing of itself.
func TestSavedItemAfterLosingAccess(t *testing.T) {
	e := newEnv(t)
	otherID, otherTok := e.createUser("mari", "Mari")
	ops := e.createChannel("ops", otherID)
	m, _ := e.send(server.DevAdminToken, ops, map[string]any{"body": "ops secret"})
	if s, _ := e.save(otherTok, map[string]any{"message_id": m.ID, "note": "check"}); s != http.StatusOK {
		t.Fatalf("save: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/channels/"+ops+"/members/"+otherID, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("remove member: %d", s)
	}
	got := e.savedList(otherTok, "")
	if len(got) != 1 || got[0].Available || got[0].Message != nil || got[0].Note != "check" {
		t.Fatalf("after losing access: %+v", got)
	}
}

// A reminder arrives once, as a system message in the person's own DM,
// with a link to the message.
func TestReminderIsSentOnce(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	meID := e.actorIDByHandle("norbi")
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "answer the invoice mail"})

	if s, _ := e.save(server.DevAdminToken, map[string]any{"message_id": m.ID, "remind_at": e.clock.Now().Add(-time.Minute)}); s != http.StatusBadRequest {
		t.Fatalf("a reminder in the past: %d", s)
	}
	at := e.clock.Now().Add(20 * time.Minute)
	if s, _ := e.save(server.DevAdminToken, map[string]any{"message_id": m.ID, "remind_at": at}); s != http.StatusOK {
		t.Fatalf("remind: %d", s)
	}
	if got := e.savedList(server.DevAdminToken, "reminders"); len(got) != 1 {
		t.Fatalf("reminders: %+v", got)
	}
	if n := e.runDueReminders(); n != 0 {
		t.Fatalf("sent before its time: %d", n)
	}
	e.clock.Advance(21 * time.Minute)
	if n := e.runDueReminders(); n != 1 {
		t.Fatalf("due reminders sent: %d", n)
	}
	if n := e.runDueReminders(); n != 0 {
		t.Fatalf("sent again: %d", n)
	}
	dm := e.selfDM(server.DevAdminToken, meID)
	if dm == nil {
		t.Fatal("no own DM")
	}
	if dm.Unread != 1 {
		t.Fatalf("the reminder is not unread: %d", dm.Unread)
	}
	msgs := e.getMessages(dm.Id.String())
	if len(msgs) != 1 || msgs[0].Kind != apispec.MessageKindSystem || !strings.Contains(msgs[0].Body, "⏰") ||
		!strings.Contains(msgs[0].Body, "[message:"+general+"/"+m.ID+"]") {
		t.Fatalf("reminder message: %+v", msgs)
	}
	got := e.savedList(server.DevAdminToken, "")
	if len(got) != 1 || got[0].RemindedAt == nil {
		t.Fatalf("not marked reminded: %+v", got)
	}

	// /remind <when> <text>: a reminder without a message.
	if s, _ := e.save(server.DevAdminToken, map[string]any{"note": "call the accountant", "remind_at": e.clock.Now().Add(time.Hour)}); s != http.StatusOK {
		t.Fatalf("plain reminder: %d", s)
	}
	if s, _ := e.save(server.DevAdminToken, map[string]any{"note": "nothing to remind"}); s != http.StatusBadRequest {
		t.Fatalf("neither message nor time: %d", s)
	}
	e.clock.Advance(61 * time.Minute)
	if n := e.runDueReminders(); n != 1 {
		t.Fatalf("plain reminder sent: %d", n)
	}
	msgs = e.getMessages(dm.Id.String())
	if last := msgs[len(msgs)-1]; !strings.Contains(last.Body, "call the accountant") || strings.Contains(last.Body, "[message:") {
		t.Fatalf("plain reminder body: %q", last.Body)
	}
}

// Several loops at once (several server instances) send a reminder once.
func TestReminderClaimIsExclusive(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "x"})
	e.save(server.DevAdminToken, map[string]any{"message_id": m.ID, "remind_at": e.clock.Now().Add(time.Minute)})
	e.clock.Advance(2 * time.Minute)
	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := e.app.Saved.RunDue(system.Context(context.Background()))
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			total += n
			mu.Unlock()
		}()
	}
	wg.Wait()
	if total != 1 {
		t.Fatalf("sent %d times", total)
	}
}

// The reminder event reaches its owner alone.
func TestSavedEventsArePersonal(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, bossTok := e.createActor("human", "boss", "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	other := e.dialWS(ctx, bossTok)
	m, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "x"})
	e.save(server.DevAdminToken, map[string]any{"message_id": m.ID})
	e.send(server.DevAdminToken, general, map[string]any{"body": "marker"})
	for _, f := range eventsUntilMessage(t, ctx, other, "marker") {
		if strings.HasPrefix(f.EventType, "saved.") {
			t.Fatalf("another admin got %s", f.EventType)
		}
	}
}
