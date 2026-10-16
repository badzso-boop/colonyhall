package guard

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// TestWSBackoffBounds verifies the exponential backoff bounds (1s to 30s).
func TestWSBackoffBounds(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: -5, want: 1 * time.Second},
		{attempt: -1, want: 1 * time.Second},
		{attempt: 0, want: 1 * time.Second},
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
		{attempt: 4, want: 16 * time.Second},
		{attempt: 5, want: 30 * time.Second},
		{attempt: 6, want: 30 * time.Second},
		{attempt: 20, want: 30 * time.Second},
		{attempt: 100, want: 30 * time.Second}, // overflow protection
	}
	for _, tc := range tests {
		got := wsBackoff(tc.attempt)
		if got != tc.want {
			t.Errorf("wsBackoff(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

type fakeTokenSource struct {
	mu    sync.Mutex
	token string
}

func (s *fakeTokenSource) Token(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token, nil
}

func (s *fakeTokenSource) setToken(t string) {
	s.mu.Lock()
	s.token = t
	s.mu.Unlock()
}

func (s *fakeTokenSource) Invalidate() {}

// TestWSTokenChangeTriggersReconnect verifies that when an agent's token changes,
// the WebSocket connection is closed and immediately re-established with the new token.
func TestWSTokenChangeTriggersReconnect(t *testing.T) {
	var mu sync.Mutex
	var receivedTokens []string
	reconnected := make(chan string, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()

		var h struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		if err := wsjson.Read(r.Context(), c, &h); err != nil {
			return
		}
		if h.Type != "hello" {
			_ = c.Close(websocket.StatusPolicyViolation, "expected hello")
			return
		}

		mu.Lock()
		receivedTokens = append(receivedTokens, h.Token)
		mu.Unlock()
		reconnected <- h.Token

		// Hold connection open until closed by client
		ctx := c.CloseRead(r.Context())
		<-ctx.Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	src := &fakeTokenSource{token: "token-v1"}
	client := &Client{
		BaseURL: server.URL,
		Tokens:  src,
	}

	wakeCh := make(chan struct{}, 10)
	agent := &Agent{
		Client:             client,
		Log:                slog.New(slog.NewTextHandler(io.Discard, nil)),
		TokenCheckInterval: 10 * time.Millisecond,
		WSBackoff:          func(int) time.Duration { return 10 * time.Millisecond },
	}

	go agent.watchWS(ctx, wakeCh)

	// Wait for the first connection with token-v1
	select {
	case tok := <-reconnected:
		if tok != "token-v1" {
			t.Fatalf("first token = %q, want token-v1", tok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial websocket connection")
	}

	// Rotate token to token-v2
	src.setToken("token-v2")

	// Wait for reconnection with token-v2
	select {
	case tok := <-reconnected:
		if tok != "token-v2" {
			t.Fatalf("second token = %q, want token-v2", tok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for reconnected websocket with new token")
	}

	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(receivedTokens) < 2 || receivedTokens[0] != "token-v1" || receivedTokens[1] != "token-v2" {
		t.Errorf("received tokens = %v, want [token-v1, token-v2]", receivedTokens)
	}
}

// TestWSWakeOnEvents verifies that event and resync_required frames trigger wake signals,
// whereas welcome and replay_done frames are ignored.
func TestWSWakeOnEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()

		var h struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		if err := wsjson.Read(r.Context(), c, &h); err != nil {
			return
		}

		// Send frames in sequence
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "welcome", "protocol": 1, "head": 10})
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "replay_done", "seq": 10})

		time.Sleep(20 * time.Millisecond)

		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "event", "seq": 11, "event_type": "message.created"})
		time.Sleep(20 * time.Millisecond)

		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "resync_required", "head": 20})
		time.Sleep(20 * time.Millisecond)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wakeCh := make(chan struct{}, 10)
	agent := &Agent{
		Client: &Client{
			BaseURL: server.URL,
			Token:   "tok-1",
		},
		Log:                slog.New(slog.NewTextHandler(io.Discard, nil)),
		TokenCheckInterval: time.Minute,
		WSBackoff:          func(int) time.Duration { return 10 * time.Millisecond },
	}

	go agent.watchWS(ctx, wakeCh)

	// Expect exactly 2 wake signals (one for event, one for resync_required)
	for i := 0; i < 2; i++ {
		select {
		case <-wakeCh:
			// Expected signal
		case <-time.After(2 * time.Second):
			t.Fatalf("signal %d did not arrive", i+1)
		}
	}

	// Ensure no extra signals arrive
	select {
	case <-wakeCh:
		t.Fatal("unexpected extra wake signal received")
	case <-time.After(50 * time.Millisecond):
		// OK
	}
}
