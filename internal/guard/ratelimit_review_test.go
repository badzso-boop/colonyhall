package guard

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// An answer that only talks about rate or usage limits is a normal answer:
// it must be posted, and it must not pause the machine's Claude agents.
func TestAgent_AnswerMentioningLimitsIsNotALimitFailure(t *testing.T) {
	ts := &ratelimitTestServer{
		inboxItems: []Item{{
			ID: "item-1", Kind: "message", ItemID: "msg-1",
			ChannelID:    "11111111-1111-1111-1111-111111111111",
			ThreadRootID: "22222222-2222-2222-2222-222222222222",
			Author:       "alice", AuthorKind: "human", Body: "status?",
		}},
	}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()

	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }
	dir := t.TempDir()
	limiter := NewClaudeLimiter(filepath.Join(dir, "usage.json"), 90, 80, nil, mockNow)

	answer := "Kész: a rate limit kijelzés és a usage limit szünet is működik; ha hit your limit jön, megáll."
	agent := &Agent{
		Cfg:           AgentConfig{Client: "claude-code", Mode: "direct", Batch: 10},
		Client:        &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Adapter:       &scriptedAdapter{directFn: func(context.Context, DirectRequest) (RunResult, error) { return RunResult{Text: answer}, nil }},
		ClaudeLimiter: limiter, Now: mockNow, StateDir: dir, Log: slog.Default(),
	}
	if err := agent.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if limiter.IsPaused(now) {
		t.Fatal("an answer mentioning limits paused the limiter")
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.notes) != 0 {
		t.Fatalf("expected no rate limit note, got %d", len(ts.notes))
	}
	if len(ts.ackedIDs) != 1 || ts.ackedIDs[0] != "item-1" {
		t.Fatalf("expected the item to be acked, got %v", ts.ackedIDs)
	}
	posted := false
	for _, r := range ts.replies {
		if r == answer {
			posted = true
		}
	}
	if !posted {
		t.Fatalf("the answer was not posted: %q", ts.replies)
	}
}

// "allowed_warning" below the reserve is not a reason to stop; "rejected" is.
func TestClaudeLimiter_AllowedWarningBelowReserveKeepsRunning(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	mockNow := func() time.Time { return now }
	limiter := NewClaudeLimiter(filepath.Join(t.TempDir(), "usage.json"), 90, 80, nil, mockNow)

	limiter.Update(RateLimit{FiveHour: &RateLimitWindow{Utilization: 0.75, ResetsAt: now.Add(time.Hour), Status: "allowed_warning"}})
	if limiter.IsPaused(now) {
		t.Fatal("allowed_warning at 75% paused the limiter")
	}
	limiter.Update(RateLimit{FiveHour: &RateLimitWindow{Utilization: 0.75, ResetsAt: now.Add(time.Hour), Status: "rejected"}})
	if !limiter.IsPaused(now) {
		t.Fatal("rejected did not pause the limiter")
	}
}

// A reply to the agent's own request still goes through the human-only gate:
// an agent's message reaches a full-access agent only after approval (as a
// "delegated" item), never just because it answers a request.
func TestHumanOnlyRefusesAgentReplyWithOriginUnlessDelegated(t *testing.T) {
	ts := &ratelimitTestServer{}
	srv := httptest.NewServer(ts.handler())
	defer srv.Close()
	agent := &Agent{
		me:     Me{ID: "bot-id", Handle: "mybot"},
		Client: &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Log:    slog.Default(),
	}
	origin := &ReplyOrigin{ChannelID: "c1", Channel: "work", ThreadRootID: "t1", RequestMessageID: "r1"}
	kept := agent.humanOnly(context.Background(), []Item{
		{ID: "it-plain", Author: "otherbot", AuthorKind: "agent", Kind: "message", DM: true, Body: "30/30", ReplyOrigin: origin},
		{ID: "it-approved", Author: "otherbot", AuthorKind: "agent", Kind: "delegated", DelegatedBy: "norbi", DM: true, Body: "30/30", ReplyOrigin: origin},
	})
	if len(kept) != 1 || kept[0].ID != "it-approved" {
		t.Fatalf("expected only the delegated reply to be kept, got %+v", kept)
	}
}
