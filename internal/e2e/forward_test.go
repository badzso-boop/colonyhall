// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/apispec"
	"colonyhall/internal/platform/clock"
	"colonyhall/internal/platform/ids"
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
	// The header is language-neutral: author, place, time (no "Továbbítva").
	if strings.Contains(body, "Továbbítva") || !strings.Contains(body, "↪ @\u200bnorbi · #general · ") {
		t.Fatalf("forward header: %q", body)
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

// download fetches a file's content: the status and the bytes.
func (e *env) download(token, fileID string) (int, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest("GET", e.srv.URL+"/api/v1/files/"+fileID+"/content", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw
}

// A forward carries the original's live attachments as copies in the
// target conversation: a reader there can open them without access to the
// source, a deleted attachment stays behind, and deleting the original
// leaves the copy (its content is stored once, shared).
func TestForwardCopiesAttachments(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	mariID, mariTok := e.createUser("mari", "Mari")
	e.addMember(general, mariID, true)
	secret := e.createChannel("secret")

	s, keep := uploadMultipart(t, e, secret, "plan.txt", []byte("the plan"), server.DevAdminToken)
	if s != http.StatusCreated {
		t.Fatalf("upload: %d", s)
	}
	_, drop := uploadMultipart(t, e, secret, "old.txt", []byte("old"), server.DevAdminToken)
	src, s := e.send(server.DevAdminToken, secret, map[string]any{"body": "files",
		"file_ids": []string{keep.Id.String(), drop.Id.String()}})
	if s != http.StatusCreated {
		t.Fatalf("send with files: %d", s)
	}
	if s := e.do("DELETE", "/api/v1/files/"+drop.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete old.txt: %d", s)
	}

	s, res := e.forward(server.DevAdminToken, src.ID, map[string]any{"channel_id": general})
	if s != http.StatusCreated || res.Message == nil || res.Message.Attachments == nil {
		t.Fatalf("forward: %d %+v", s, res.Message)
	}
	atts := *res.Message.Attachments
	if len(atts) != 1 || atts[0].Name != "plan.txt" || atts[0].Size != int64(len("the plan")) {
		t.Fatalf("forwarded attachments = %+v, want only plan.txt", atts)
	}
	cp := atts[0].Id.String()
	if cp == keep.Id.String() {
		t.Fatal("the forward reused the source file instead of a copy in the target conversation")
	}
	// Mari reads general, not secret: the copy opens, the original does not.
	if s, body := e.download(mariTok, cp); s != http.StatusOK || string(body) != "the plan" {
		t.Fatalf("copy for a reader of the target: %d %q", s, body)
	}
	if s, _ := e.download(mariTok, keep.Id.String()); s == http.StatusOK {
		t.Fatal("the original opened for someone outside its conversation")
	}
	// Deleting the original keeps the copy and its content.
	if s := e.do("DELETE", "/api/v1/files/"+keep.Id.String(), server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("delete the original: %d", s)
	}
	if s, body := e.download(mariTok, cp); s != http.StatusOK || string(body) != "the plan" {
		t.Fatalf("copy after the original was deleted: %d %q", s, body)
	}
}

// TestPostgresForwardCopiesAttachments: the copies are written in the
// forward's own transaction (content lock, usage row, event), on Postgres
// too. Runs with `make test-pg`; the database outlives the test, so the
// names and the content are unique per run.
func TestPostgresForwardCopiesAttachments(t *testing.T) {
	url := os.Getenv("COLONY_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("COLONY_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	clk := clock.NewManual(time.Now().UTC())
	app, err := server.New(ctx, server.Options{Store: "postgres", DatabaseURL: url, Clock: clk, IDs: ids.V7{}, FilesDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Close)
	if err := app.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(app.Handler)
	t.Cleanup(srv.Close)
	e := &env{t: t, app: app, srv: srv, router: specRouter(t, srv.URL), clock: clk}
	e.do("PUT", "/api/v1/workspace/storage/quota", server.DevAdminToken, map[string]any{"quota_bytes": 0}, nil)

	run := fmt.Sprint(clk.Now().UnixNano())
	from, to := e.createChannel("fwd-from-"+run), e.createChannel("fwd-to-"+run)
	content := []byte("forwarded " + run)
	if s, f := uploadMultipart(t, e, from, "a.txt", content, server.DevAdminToken); s != http.StatusCreated {
		t.Fatalf("upload: %d", s)
	} else {
		src, s := e.send(server.DevAdminToken, from, map[string]any{"body": "file", "file_ids": []string{f.Id.String()}})
		if s != http.StatusCreated {
			t.Fatalf("send: %d", s)
		}
		s, res := e.forward(server.DevAdminToken, src.ID, map[string]any{"channel_id": to})
		if s != http.StatusCreated || res.Message == nil || res.Message.Attachments == nil || len(*res.Message.Attachments) != 1 {
			t.Fatalf("forward: %d %+v", s, res.Message)
		}
		cp := (*res.Message.Attachments)[0].Id.String()
		if cp == f.Id.String() {
			t.Fatal("the forward reused the source file")
		}
		if s, body := e.download(server.DevAdminToken, cp); s != http.StatusOK || string(body) != string(content) {
			t.Fatalf("the copy: %d %q", s, body)
		}
	}
}
