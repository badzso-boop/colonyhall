package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNotesDelivery_FreshVsResumed(t *testing.T) {
	noteText := "Task: optimize sql query. State: analyzing plan."

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/agents/") && strings.HasSuffix(r.URL.Path, "/notes"):
			if noteText == "" {
				_ = json.NewEncoder(w).Encode(map[string]any{"notes": []any{}})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"notes": []map[string]any{
						{
							"id":             "00000000-0000-0000-0000-000000000001",
							"agent_id":       "00000000-0000-0000-0000-000000000002",
							"thread_root_id": "00000000-0000-0000-0000-000000000003",
							"text":           noteText,
							"created_at":     time.Now().Format(time.RFC3339),
						},
					},
				})
			}
		case strings.HasSuffix(r.URL.Path, "/api/v1/agent/instructions"):
			_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "text": "Rules"})
		case strings.HasSuffix(r.URL.Path, "/inbox/ack"):
			_ = json.NewEncoder(w).Encode(map[string]any{"acked": 1})
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "msg-reply"})
		case strings.HasSuffix(r.URL.Path, "/runs"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "run-1"})
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	var capturedPrompts []string

	ag := &Antigravity{
		Run: func(ctx context.Context, dir, name string, args, env []string) ([]byte, []byte, error) {
			p, _ := flagValue(args, "-p")
			capturedPrompts = append(capturedPrompts, p)
			return []byte(`{"status":"SUCCESS","response":"ok","conversation_id":"conv-1"}`), nil, nil
		},
	}
	stateDir := t.TempDir()
	agent := &Agent{
		Cfg: AgentConfig{
			Client:    "antigravity",
			Name:      "test-agent",
			HaltCheck: time.Hour,
		},
		Client:   &Client{BaseURL: ts.URL, Token: "tok-agent", HTTP: ts.Client()},
		Adapter:  ag,
		StateDir: stateDir,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
	}

	// Run 1: First run (fresh session) -> must include <colonyhall_note>
	item1 := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", Channel: "general", ChannelID: "c1", Body: "hello"}}
	if err := agent.handleDirect(ctx, item1); err != nil {
		t.Fatal(err)
	}
	if len(capturedPrompts) != 1 {
		t.Fatalf("expected 1 prompt captured, got %d", len(capturedPrompts))
	}
	p1 := capturedPrompts[0]
	if !strings.Contains(p1, "<colonyhall_note>") || !strings.Contains(p1, noteText) {
		t.Fatalf("run 1 (fresh): expected prompt to contain <colonyhall_note>, got:\n%s", p1)
	}

	// Run 2: Resumed run on same thread -> must NOT include <colonyhall_note>
	item2 := []Item{{ID: "it-2", AuthorKind: "human", ItemID: "m2", ThreadRootID: "m1", Channel: "general", ChannelID: "c1", Body: "continue"}}
	if err := agent.handleDirect(ctx, item2); err != nil {
		t.Fatal(err)
	}
	if len(capturedPrompts) != 2 {
		t.Fatalf("expected 2 prompts captured, got %d", len(capturedPrompts))
	}
	p2 := capturedPrompts[1]
	if strings.Contains(p2, "<colonyhall_note>") {
		t.Fatalf("run 2 (resumed): expected prompt NOT to contain <colonyhall_note>, got:\n%s", p2)
	}
}
