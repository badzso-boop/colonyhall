package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// A request another agent sent from a thread (request_origin) runs in that
// thread's session, and the answer is posted there, linked to the request
// (2026-11-10: the answer stayed in the DM, and the thread did not know it).
func TestRequestOriginDirectExecution(t *testing.T) {
	base := t.TempDir()
	stateDir := filepath.Join(base, "state")
	root := filepath.Join(base, "project")
	_ = os.MkdirAll(stateDir, 0o700)
	_ = os.MkdirAll(root, 0o700)
	agentID := uuid.New().String()

	var mu sync.Mutex
	var sentMessages []struct {
		ChannelID    string `json:"channel_id"`
		ThreadRootID string `json:"thread_root_id"`
		Body         string `json:"body"`
		Answers      string `json:"answers_message_id"`
	}
	var reactions []string
	var ackedItems []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/v1/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "handle": "bot"})
		case r.Method == "GET" && r.URL.Path == "/api/v1/auth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "new-tok", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339)})
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/v1/channels/") && strings.HasSuffix(r.URL.Path, "/messages"):
			var in struct {
				ThreadRootID string `json:"thread_root_id"`
				Body         string `json:"body"`
				Answers      string `json:"answers_message_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(r.URL.Path, "/")
			chanID := parts[4]
			sentMessages = append(sentMessages, struct {
				ChannelID    string `json:"channel_id"`
				ThreadRootID string `json:"thread_root_id"`
				Body         string `json:"body"`
				Answers      string `json:"answers_message_id"`
			}{ChannelID: chanID, ThreadRootID: in.ThreadRootID, Body: in.Body, Answers: in.Answers})
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"id": "msg-reply-1"}})
		case r.Method == "PUT" && strings.Contains(r.URL.Path, "/reactions/"):
			reactions = append(reactions, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "POST" && r.URL.Path == "/api/v1/agent/ack":
			var in struct {
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			ackedItems = append(ackedItems, in.IDs...)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "POST" && r.URL.Path == "/api/v1/agent/runs":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "run-1"})
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer ts.Close()

	var directReq DirectRequest
	var mcpBytes []byte
	ad := &directTestAdapter{
		impl: func(ctx context.Context, req DirectRequest) (RunResult, error) {
			directReq = req
			if req.MCPConfigPath != "" {
				mcpBytes, _ = os.ReadFile(req.MCPConfigPath)
			}
			return RunResult{Text: "Done: the component is in the layout.", SessionID: req.SessionID}, nil
		},
	}

	agent := &Agent{
		Cfg: AgentConfig{
			Client:    "claude-code",
			HaltCheck: time.Hour,
		},
		Client:   &Client{BaseURL: ts.URL, Token: "tok-1", HTTP: ts.Client()},
		Adapter:  ad,
		StateDir: stateDir,
		RootDir:  root,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
		me:       Me{ID: agentID, Handle: "bot"},
	}

	origChanID := uuid.New().String()
	origThreadID := uuid.New().String()
	dmChanID := uuid.New().String()
	dmMsgID := uuid.New().String()
	reqMsgID := uuid.New().String()

	item := Item{
		ID:          "it-request-1",
		Kind:        "delegated",
		DelegatedBy: "norbi",
		ItemID:      dmMsgID,
		ChannelID:   dmChanID,
		DM:          true,
		Author:      "otherbot",
		AuthorKind:  "agent",
		Body:        "Please implement the Umami component.",
		RequestOrigin: &ReplyOrigin{
			ChannelID:        origChanID,
			Channel:          "homelab",
			ThreadRootID:     origThreadID,
			RequestMessageID: dmMsgID,
		},
	}
	_ = reqMsgID

	expectedSessionID, _ := DirectSessionID(agentID, origThreadID)

	if err := agent.handleDirect(context.Background(), []Item{item}); err != nil {
		t.Fatalf("handleDirect failed: %v", err)
	}

	// 1. The request runs in the origin thread's session: a later question
	// in that thread reaches the session that saw the request.
	if directReq.SessionID != expectedSessionID {
		t.Fatalf("expected session ID %s, got %s", expectedSessionID, directReq.SessionID)
	}

	// 2. The answer goes into the origin thread, where the people who asked
	// see it, linked to the DM request.
	mu.Lock()
	defer mu.Unlock()
	if len(sentMessages) != 1 {
		t.Fatalf("expected 1 sent message, got %d: %+v", len(sentMessages), sentMessages)
	}
	reply := sentMessages[0]
	if reply.ChannelID != origChanID {
		t.Fatalf("reply posted to channel %s, want origin %s", reply.ChannelID, origChanID)
	}
	if reply.ThreadRootID != origThreadID {
		t.Fatalf("reply posted to thread %s, want origin thread %s", reply.ThreadRootID, origThreadID)
	}
	if reply.Answers != dmMsgID {
		t.Fatalf("reply answers_message_id = %q, want the DM request %s", reply.Answers, dmMsgID)
	}
	if strings.Contains(reply.Body, "dm_reply") || strings.Contains(reply.Body, dmChanID) {
		t.Fatalf("reply body carries routing markup: %s", reply.Body)
	}

	// 3. 👍 on the DM request
	if len(reactions) != 1 || !strings.Contains(reactions[0], dmMsgID) {
		t.Fatalf("expected 👍 reaction on DM message %s, got: %v", dmMsgID, reactions)
	}

	// 4. MCP config headers contained X-Colonyhall-Thread: origThreadID
	if len(mcpBytes) == 0 {
		t.Fatal("MCP config was not read during run")
	}
	var mcpCfg struct {
		MCPServers map[string]struct {
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(mcpBytes, &mcpCfg); err != nil {
		t.Fatalf("failed to unmarshal MCP config: %v", err)
	}
	chHeaders := mcpCfg.MCPServers["colonyhall"].Headers
	if chHeaders["X-Colonyhall-Channel"] != origChanID {
		t.Fatalf("MCP header X-Colonyhall-Channel: got %q, want %q", chHeaders["X-Colonyhall-Channel"], origChanID)
	}
	if chHeaders["X-Colonyhall-Thread"] != origThreadID {
		t.Fatalf("MCP header X-Colonyhall-Thread: got %q, want %q", chHeaders["X-Colonyhall-Thread"], origThreadID)
	}
}

func TestRequestOriginThreadRootAndPrompt(t *testing.T) {
	it := Item{ID: "it-1", ChannelID: "dm-1", ItemID: "dm-msg-1", DM: true, Author: "agy-homelab", AuthorKind: "agent",
		Body:          "Please implement it.",
		RequestOrigin: &ReplyOrigin{ChannelID: "chan-1", Channel: "homelab", ThreadRootID: "root-1", RequestMessageID: "dm-msg-1"}}
	if root := ThreadRoot(it); root != "root-1" {
		t.Fatalf("ThreadRoot: %q, want the origin thread", root)
	}
	if r := it.Route(); r == nil || r.ThreadRootID != "root-1" {
		t.Fatalf("Route: %+v", r)
	}
	prompt := DirectPrompt([]Item{it})
	if !strings.Contains(prompt, `sent_from_thread="#homelab"`) || !strings.Contains(prompt, "your answer is posted in that thread") {
		t.Fatalf("prompt: %s", prompt)
	}
	if strings.Contains(prompt, "answers_your_request") {
		t.Fatalf("a request is not an answer: %s", prompt)
	}
}
