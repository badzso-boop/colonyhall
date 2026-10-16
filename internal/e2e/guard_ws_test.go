package e2e

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

// TestGuardWebSocketWakeReactsFast verifies that a guard agent with an active
// WebSocket connection reacts to a mention almost immediately (< 2s), well before
// the 1-minute fallback_poll interval.
func TestGuardWebSocketWakeReactsFast(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	ad := &scriptedAdapter{
		decide: func(_ context.Context, req guard.DecideRequest) (guard.Decision, error) {
			return guard.Decision{Actions: []guard.Action{
				{Ref: "1", Kind: guard.ActReply, Text: "fast pong"},
			}}, nil
		},
	}

	a := &guard.Agent{
		Cfg: guard.AgentConfig{
			Client:       "claude-code",
			Debounce:     time.Millisecond,
			HaltCheck:    5 * time.Millisecond,
			FallbackPoll: 60 * time.Second, // 1 minute fallback
		},
		Client:  &guard.Client{BaseURL: e.srv.URL, Token: server.DevAgentToken},
		Adapter: ad,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	go func() { _ = a.Run(ctx) }()

	// Allow the agent to start and establish the WebSocket connection.
	time.Sleep(50 * time.Millisecond)

	general := e.generalID()
	start := time.Now()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent ping fast"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	eventually(t, "reply via websocket wake", func() bool {
		msgs := e.messages(general)
		if len(msgs) == 0 {
			return false
		}
		last := msgs[len(msgs)-1]
		return last.ThreadRootID == q.ID && last.Body == "fast pong"
	})

	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("agent reacted in %v, expected < 2s (fallback_poll is 60s)", elapsed)
	}
}

// TestGuardFallbackPollPicksUpWhenWSUnavailable verifies that if WebSocket is
// unavailable (e.g. invalid token for WS only via test hook), the fallback_poll
// timer still picks up inbox items and answers them.
func TestGuardFallbackPollPicksUpWhenWSUnavailable(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	ad := &scriptedAdapter{
		decide: func(_ context.Context, req guard.DecideRequest) (guard.Decision, error) {
			return guard.Decision{Actions: []guard.Action{
				{Ref: "1", Kind: guard.ActReply, Text: "fallback pong"},
			}}, nil
		},
	}

	a := &guard.Agent{
		Cfg: guard.AgentConfig{
			Client:       "claude-code",
			Debounce:     time.Millisecond,
			HaltCheck:    5 * time.Millisecond,
			FallbackPoll: 50 * time.Millisecond, // short fallback interval for test
		},
		Client: &guard.Client{
			BaseURL: e.srv.URL,
			Token:   server.DevAgentToken,
			WSTokenHook: func(context.Context) (string, error) {
				return "invalid-ws-token", nil // rejected by /ws/ui
			},
		},
		Adapter:   ad,
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		WSBackoff: func(int) time.Duration { return 50 * time.Millisecond },
	}

	go func() { _ = a.Run(ctx) }()

	// Wait for agent to start up and fail initial WS connect
	time.Sleep(50 * time.Millisecond)

	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent ping fallback"})
	if err := e.app.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	eventually(t, "reply via fallback poll", func() bool {
		msgs := e.messages(general)
		if len(msgs) == 0 {
			return false
		}
		last := msgs[len(msgs)-1]
		return last.ThreadRootID == q.ID && last.Body == "fallback pong"
	})
}
