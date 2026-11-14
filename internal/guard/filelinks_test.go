// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestFileLinks(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "empty",
			text: "",
			want: nil,
		},
		{
			name: "hungarian text with trailing comma and dot",
			text: "A logfájl elkészült: /home/alice/logs/scheduler.log, nézd meg! További részletek: /var/log/app.log.",
			want: []string{"/home/alice/logs/scheduler.log", "/var/log/app.log"},
		},
		{
			name: "markdown file URL and bare path",
			text: "Lásd az eredményt [itt](file:///home/alice/report.md) vagy [itt](/mnt/data/stats.csv).",
			want: []string{"/home/alice/report.md", "/mnt/data/stats.csv"},
		},
		{
			name: "code spans with backticks",
			text: "A kimenet a `/tmp/script.sh` és `/opt/myapp/output.json` fájlokban van.",
			want: []string{"/tmp/script.sh", "/opt/myapp/output.json"},
		},
		{
			name: "links at the very end of text",
			text: "A végleges riport itt érhető el: /srv/reports/final.pdf",
			want: []string{"/srv/reports/final.pdf"},
		},
		{
			name: "two links on one line",
			text: "Összehasonlítás: /home/user/a.txt és /home/user/b.txt között.",
			want: []string{"/home/user/a.txt", "/home/user/b.txt"},
		},
		{
			name: "web URLs must not match",
			text: "Lásd: https://github.com/home/user/repo és http://example.com/mnt/data vagy https://host.domain:8080/tmp/x.",
			want: nil,
		},
		{
			name: "URL escapes in file URLs",
			text: "Elérési út: file:///home/alice/my%20documents/project%20report.md",
			want: []string{"/home/alice/my documents/project report.md"},
		},
		{
			name: "duplicates and order preservation",
			text: "Először /home/user/a.txt, aztán /mnt/b.txt, majd újra file:///home/user/a.txt és /mnt/b.txt.",
			want: []string{"/home/user/a.txt", "/mnt/b.txt"},
		},
		{
			name: "home tilde prefix",
			text: "A profil konfigurációja: ~/app_config.json található.",
			want: []string{"~/app_config.json"},
		},
		{
			name: "quotes double and single and brackets",
			text: "Fájlok: \"/root/setup.log\", '/tmp/cache.db', és </var/data.bin>.",
			want: []string{"/root/setup.log", "/tmp/cache.db", "/var/data.bin"},
		},
		{
			name: "bare prefix without file path is ignored",
			text: "Csak a mappa neve: /home/ és ~/ önmagában.",
			want: nil,
		},
		{
			name: "file links with line and range suffixes",
			text: "Nézd meg: /home/alice/logs/scheduler.log:42-58, /var/log/app.log:12 és file:///tmp/test.go:10-20.",
			want: []string{"/home/alice/logs/scheduler.log", "/var/log/app.log", "/tmp/test.go"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FileLinks(tc.text)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("FileLinks(%q) = %v; want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestReplyLostLinks(t *testing.T) {
	type sentMsg struct {
		ChannelID string
		ThreadID  string
		Body      string
		ClientID  string
		Elaborate bool
	}

	var sent []sentMsg
	failFollowUp := false

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/messages") {
			var payload struct {
				Body         string `json:"body"`
				ClientMsgID  string `json:"client_msg_id"`
				ThreadRootID string `json:"thread_root_id"`
				Elaborate    bool   `json:"elaborate"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if failFollowUp && strings.HasSuffix(payload.ClientMsgID, "-files") {
				http.Error(w, "server error", 500)
				return
			}
			parts := strings.Split(r.URL.Path, "/")
			channelID := parts[4] // /api/v1/channels/{channelID}/messages
			sent = append(sent, sentMsg{
				ChannelID: channelID,
				ThreadID:  payload.ThreadRootID,
				Body:      payload.Body,
				ClientID:  payload.ClientMsgID,
				Elaborate: payload.Elaborate,
			})
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg-123"})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	agent := &Agent{
		Client: &Client{
			BaseURL: ts.URL,
			HTTP:    ts.Client(),
		},
		Cfg: AgentConfig{Name: "test-bot"},
	}

	ctx := context.Background()

	// 1. A 9000-character answer ending with a path -> clipped reply + follow-up with exactly that path
	sent = nil
	longText := strings.Repeat("A", 8500) + "\nA végeredmény itt van: /home/user/final_report.txt"
	item := Item{ChannelID: "chan-1", ThreadRootID: "thread-1"}
	if err := agent.replyLong(ctx, item, longText, "client-1"); err != nil {
		t.Fatalf("replyLong failed: %v", err)
	}

	if len(sent) != 2 {
		t.Fatalf("expected 2 sent messages, got %d", len(sent))
	}
	if strings.Contains(sent[0].Body, "/home/user/final_report.txt") {
		t.Errorf("first message should have been clipped: %s", sent[0].Body[len(sent[0].Body)-50:])
	}
	if sent[0].ClientID != "client-1" {
		t.Errorf("first clientID = %q, want client-1", sent[0].ClientID)
	}
	if sent[1].ClientID != "client-1-files" {
		t.Errorf("second clientID = %q, want client-1-files", sent[1].ClientID)
	}
	wantFollowUp := "📎\n/home/user/final_report.txt"
	if sent[1].Body != wantFollowUp {
		t.Errorf("follow-up body = %q, want %q", sent[1].Body, wantFollowUp)
	}

	// 2. Short answer -> no follow-up
	sent = nil
	shortText := "Itt a fájl: /home/user/summary.txt"
	if err := agent.reply(ctx, item, shortText, "client-2"); err != nil {
		t.Fatalf("reply failed: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message for short text, got %d", len(sent))
	}
	if sent[0].ClientID != "client-2" {
		t.Errorf("clientID = %q, want client-2", sent[0].ClientID)
	}

	// 3. A path cut in half by clip counts as lost
	sent = nil
	// MaxReplyLen is 600. Put 585 runes of padding, then a 30 rune path.
	cutPath := "/home/alice/logs/scheduler.log"
	cutText := strings.Repeat("x", 585) + " " + cutPath
	if err := agent.reply(ctx, item, cutText, "client-3"); err != nil {
		t.Fatalf("reply failed: %v", err)
	}
	if len(sent) != 2 {
		t.Fatalf("expected 2 sent messages for cut path, got %d", len(sent))
	}
	if strings.Contains(sent[0].Body, cutPath) {
		t.Errorf("clipped message should not contain full path: %s", sent[0].Body)
	}
	if sent[1].ClientID != "client-3-files" {
		t.Errorf("follow-up clientID = %q, want client-3-files", sent[1].ClientID)
	}
	if sent[1].Body != "📎\n"+cutPath {
		t.Errorf("follow-up body = %q, want %q", sent[1].Body, "📎\n"+cutPath)
	}

	// 4. Follow-up failure is logged and does not fail the run
	sent = nil
	failFollowUp = true
	if err := agent.replyLong(ctx, item, longText, "client-4"); err != nil {
		t.Fatalf("replyLong should not fail when follow-up fails: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent message when follow-up fails, got %d", len(sent))
	}
}
