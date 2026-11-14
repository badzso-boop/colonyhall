package e2e

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/server"
)

func (e *env) drafts(token string) []apispec.Draft {
	e.t.Helper()
	var out apispec.DraftList
	if s := e.do("GET", "/api/v1/me/drafts", token, nil, &out); s != http.StatusOK {
		e.t.Fatalf("list drafts: %d", s)
	}
	return out.Drafts
}

// Round 55, part A: a draft is per person, conversation and thread, only its
// owner reads it, and it is gone once its owner sends there.
func TestDraftsArePrivatePerPlace(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, otherTok := e.createUser("mari", "Mari")
	e.addMember(general, e.actorIDByHandle("mari"), true)

	put := func(token string, body map[string]any) (int, apispec.Draft) {
		var d apispec.Draft
		return e.do("PUT", "/api/v1/me/drafts", token, body, &d), d
	}
	if s, d := put(server.DevAdminToken, map[string]any{"channel_id": general, "body": "half a thought"}); s != http.StatusOK || d.Body != "half a thought" {
		t.Fatalf("save: %d %+v", s, d)
	}
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "a thread root"})
	if s, _ := put(server.DevAdminToken, map[string]any{"channel_id": general, "thread_root_id": root.ID, "body": "a reply draft"}); s != http.StatusOK {
		t.Fatalf("save thread draft: %d", s)
	}
	mine := e.drafts(server.DevAdminToken)
	if len(mine) != 2 {
		t.Fatalf("want a main and a thread draft, got %+v", mine)
	}

	// Another person, an agent: nothing, and no way in.
	if got := e.drafts(otherTok); len(got) != 0 {
		t.Fatalf("another person sees drafts: %+v", got)
	}
	if s := e.do("GET", "/api/v1/me/drafts", server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		t.Fatalf("agent list: %d", s)
	}
	if s, _ := put(server.DevAgentToken, map[string]any{"channel_id": general, "body": "x"}); s != http.StatusForbidden {
		t.Fatalf("agent save: %d", s)
	}
	// Mari's delete of the same place removes only her own (there is none).
	if s := e.do("DELETE", "/api/v1/me/drafts?channel_id="+general, otherTok, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete: %d", s)
	}
	if len(e.drafts(server.DevAdminToken)) != 2 {
		t.Fatal("another person's delete removed my draft")
	}

	// A conversation the person cannot read is not found.
	secret := e.createChannel("secret")
	if s, _ := put(otherTok, map[string]any{"channel_id": secret, "body": "x"}); s != http.StatusNotFound {
		t.Fatalf("draft in an unreadable channel: %d", s)
	}
	// A thread root from another channel is refused.
	if s, _ := put(server.DevAdminToken, map[string]any{"channel_id": secret, "thread_root_id": root.ID, "body": "x"}); s != http.StatusBadRequest {
		t.Fatalf("foreign thread root: %d", s)
	}

	// Emptying the composer deletes it.
	if s, _ := put(server.DevAdminToken, map[string]any{"channel_id": general, "thread_root_id": root.ID, "body": "  "}); s != http.StatusNoContent {
		t.Fatalf("empty save: %d", s)
	}
	if got := e.drafts(server.DevAdminToken); len(got) != 1 || got[0].ThreadRootId != nil {
		t.Fatalf("after emptying the thread draft: %+v", got)
	}

	// Sending in that place (from any device) clears it.
	e.send(server.DevAdminToken, general, map[string]any{"body": "the finished thought"})
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.drafts(server.DevAdminToken); len(got) != 0 {
		t.Fatalf("draft kept after sending: %+v", got)
	}
}

// A draft saved after a message of its owner is the next one: the send's
// clean-up leaves it.
func TestDraftNewerThanTheSendStays(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	e.send(server.DevAdminToken, general, map[string]any{"body": "sent"})
	e.clock.Advance(time.Second)
	if s := e.do("PUT", "/api/v1/me/drafts", server.DevAdminToken, map[string]any{"channel_id": general, "body": "next"}, nil); s != http.StatusOK {
		t.Fatalf("save: %d", s)
	}
	if err := e.app.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.drafts(server.DevAdminToken); len(got) != 1 {
		t.Fatalf("the newer draft went: %+v", got)
	}
}

// Drafts hold only the person's own uploads in that conversation.
func TestDraftFiles(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, otherTok := e.createUser("mari", "Mari")
	e.addMember(general, e.actorIDByHandle("mari"), true)
	s, mine := uploadMultipart(t, e, general, "plan.txt", []byte("plan"), server.DevAdminToken)
	if s != http.StatusCreated {
		t.Fatalf("upload: %d", s)
	}
	_, theirs := uploadMultipart(t, e, general, "theirs.txt", []byte("theirs"), otherTok)

	var d apispec.Draft
	if s := e.do("PUT", "/api/v1/me/drafts", server.DevAdminToken, map[string]any{"channel_id": general, "body": "", "file_ids": []string{mine.Id.String()}}, &d); s != http.StatusOK {
		t.Fatalf("file-only draft: %d", s)
	}
	if len(d.Attachments) != 1 || d.Attachments[0].Name != "plan.txt" {
		t.Fatalf("attachments: %+v", d.Attachments)
	}
	if s := e.do("PUT", "/api/v1/me/drafts", server.DevAdminToken, map[string]any{"channel_id": general, "body": "x", "file_ids": []string{theirs.Id.String()}}, nil); s != http.StatusBadRequest {
		t.Fatalf("someone else's upload: %d", s)
	}
}

// The event a draft makes carries no text and reaches its owner alone, not
// even another admin.
func TestDraftEventsReachOnlyTheOwner(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, bossTok := e.createActor("human", "boss", "admin")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	other := e.dialWS(ctx, bossTok)
	owner := e.dialWS(ctx, server.DevAdminToken)

	const secret = "a very private half sentence"
	if s := e.do("PUT", "/api/v1/me/drafts", server.DevAdminToken, map[string]any{"channel_id": general, "body": secret}, nil); s != http.StatusOK {
		t.Fatalf("save: %d", s)
	}
	e.send(server.DevAdminToken, general, map[string]any{"body": "marker"})

	for _, f := range eventsUntilMessage(t, ctx, other, "marker") {
		if f.EventType == "draft.changed" {
			t.Fatal("another admin got the draft event")
		}
	}
	seen := false
	for _, f := range eventsUntilMessage(t, ctx, owner, "marker") {
		if f.EventType == "draft.changed" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("the owner's other tabs did not hear of the draft")
	}

	events, err := e.app.Events.Since(context.Background(), 0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if bytes.Contains(ev.Payload, []byte(secret)) {
			t.Fatalf("the draft text is in the event log (%s)", ev.Type)
		}
	}
}
