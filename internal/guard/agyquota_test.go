package guard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const quotaReason = "RESOURCE_EXHAUSTED (code 429): Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 6h43m54s."

// The result event agy prints for a failed run (shape seen with an invalid
// model name on agy 1.4.12: the reason is in "error", "response" is empty).
func agyFailedResult(reason string) string {
	return fmt.Sprintf(`{"event":"result","result":{"conversation_id":"","status":"ERROR","response":"","error":%q,"usage":{"input_tokens":0,"output_tokens":0}}}`, reason)
}

func TestAntigravityKeepsTheErrorField(t *testing.T) {
	a := &Antigravity{StreamRun: func(_ context.Context, _, _ string, _, _ []string, line func(string)) error {
		line(agyFailedResult(quotaReason))
		return nil
	}}
	_, err := a.Direct(context.Background(), DirectRequest{Prompt: "hi", Dir: t.TempDir()})
	var re *AgyRunError
	if !errors.As(err, &re) || !strings.Contains(re.Reason, "Individual quota reached") {
		t.Fatalf("stream: %v", err)
	}
	if !strings.Contains(err.Error(), "Individual quota reached") {
		t.Fatalf("the run error must carry the reason: %v", err)
	}

	b := &Antigravity{Run: func(context.Context, string, string, []string, []string) ([]byte, []byte, error) {
		return []byte(`{"status":"ERROR","response":"","error":"` + quotaReason + `"}`), nil, nil
	}}
	_, err = b.Decide(context.Background(), DecideRequest{Handle: "x", Dir: t.TempDir()})
	if !errors.As(err, &re) || !strings.Contains(re.Reason, "quota reached") {
		t.Fatalf("json: %v", err)
	}
}

func TestParseAgyReset(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"… Resets in 6h43m54s.": 6*time.Hour + 43*time.Minute + 54*time.Second,
		"Resets in 15m":         15 * time.Minute,
		"resets in 2h":          2 * time.Hour,
		"Resets in 30s":         30 * time.Second,
	} {
		if got, ok := parseAgyReset(in); !ok || got != want {
			t.Errorf("%q: %v %v", in, got, ok)
		}
	}
	if _, ok := parseAgyReset("Individual quota reached."); ok {
		t.Error("no reset time must not parse")
	}
}

func writeLog(t *testing.T, dir string, at time.Time, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, "cli-"+at.Format("20060102_150405")+".log")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAgyLogReason(t *testing.T) {
	dir := t.TempDir()
	started := time.Now().Truncate(time.Second)
	noise := []string{
		"I1101 09:14:03.223216 1 server_oauth.go:224] applyAuthResult: email=someone@example.com, authMethod=consumer",
		"W1101 09:14:03.300000 1 auth.go:88] You are not logged into Antigravity",
		"W1101 09:14:03.400000 1 api.go:12] 403 insufficient authentication scopes",
		"E1101 09:14:12.844296 621 errorreport.go:224] agent executor error: generating and executing: " + quotaReason,
	}
	// An older run's log, with another error: never read.
	writeLog(t, dir, started.Add(-time.Hour), "E1101 08:00:00.0 1 session.go:280] Print mode: run ended with error and no response: something else")
	writeLog(t, dir, started.Add(time.Second), append(noise, "E1101 09:14:12.875254 1 session.go:280] Print mode: run ended with error and no response: Individual quota reached. Resets in 6h43m54s.")...)

	got := agyLogReason(dir, started)
	if got != "Individual quota reached. Resets in 6h43m54s." {
		t.Fatalf("reason: %q", got)
	}
	if strings.Contains(got, "@") {
		t.Fatal("nothing but the error line may leave the log")
	}

	// A log that is only a link is not followed.
	outside := filepath.Join(t.TempDir(), "secret.log")
	_ = os.WriteFile(outside, []byte("run ended with error and no response: from outside\n"), 0o600)
	linkDir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(linkDir, "cli-"+started.Add(time.Second).Format("20060102_150405")+".log")); err != nil {
		t.Fatal(err)
	}
	if got := agyLogReason(linkDir, started); got != "" {
		t.Fatalf("followed a symlink: %q", got)
	}

	// Two parallel runs with different reasons: no guess.
	writeLog(t, dir, started.Add(2*time.Second), "E 1 session.go:280] Print mode: run ended with error and no response: another reason")
	if got := agyLogReason(dir, started); got != "" {
		t.Fatalf("picked one of two reasons: %q", got)
	}

	// Without the session line, the RESOURCE_EXHAUSTED line is the reason.
	only := t.TempDir()
	writeLog(t, only, started, noise...)
	if got := agyLogReason(only, started); !strings.HasPrefix(got, "RESOURCE_EXHAUSTED (code 429)") {
		t.Fatalf("exhausted line: %q", got)
	}
}

type agyAgentSetup struct {
	ts      *ratelimitTestServer
	agent   *Agent
	limiter *AgyLimiter
	now     *time.Time
	runs    *int
	models  *[]string
}

func newAgyAgent(t *testing.T, model string, limiter *AgyLimiter, now *time.Time, fail func(n int) (RunResult, error)) agyAgentSetup {
	t.Helper()
	ts := &ratelimitTestServer{inboxItems: []Item{{
		ID: "item-" + model, Kind: "message", ItemID: "msg-1", ChannelID: "11111111-1111-1111-1111-111111111111",
		ThreadRootID: "22222222-2222-2222-2222-222222222222", Author: "alice", AuthorKind: "human", Body: "run task",
	}}}
	srv := httptest.NewServer(ts.handler())
	t.Cleanup(srv.Close)
	runs := 0
	var models []string
	adapter := &scriptedAdapter{directFn: func(_ context.Context, req DirectRequest) (RunResult, error) {
		runs++
		models = append(models, req.Model)
		return fail(runs)
	}}
	agent := &Agent{
		Cfg:        AgentConfig{Client: "antigravity", Mode: "direct", Batch: 10, Model: model, WorkModel: model},
		Client:     &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Adapter:    adapter,
		AgyLimiter: limiter,
		Now:        func() time.Time { return *now },
		StateDir:   t.TempDir(),
		Log:        slog.Default(),
	}
	return agyAgentSetup{ts: ts, agent: agent, limiter: limiter, now: now, runs: &runs, models: &models}
}

func TestAgyQuotaPausesTheModelOnly(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 14, 0, 0, time.UTC)
	state := filepath.Join(t.TempDir(), "agy_quota.json")
	limiter := NewAgyLimiter(state, nil, func() time.Time { return now })
	quota := func(int) (RunResult, error) {
		return RunResult{}, &AgyRunError{Status: "ERROR", Reason: quotaReason}
	}
	flash := newAgyAgent(t, "gemini-3.8-flash-high", limiter, &now, quota)
	ctx := context.Background()

	if err := flash.agent.Tick(ctx); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if *flash.runs != 1 || flash.agent.attempts["item-gemini-3.8-flash-high"] != 0 {
		t.Fatalf("runs %d, attempts %d: a quota error must not count", *flash.runs, flash.agent.attempts["item-gemini-3.8-flash-high"])
	}
	if len(flash.ts.ackedIDs) != 0 {
		t.Fatalf("the item must stay pending: acked %v", flash.ts.ackedIDs)
	}
	if len(flash.ts.notes) != 1 || flash.ts.notes[0].Window != "agy_quota" || flash.ts.notes[0].Model == nil || *flash.ts.notes[0].Model != "gemini-3.8-flash-high" {
		t.Fatalf("notes: %+v", flash.ts.notes)
	}
	wantUntil := now.Add(6*time.Hour + 43*time.Minute + 54*time.Second + time.Minute)
	if !flash.ts.notes[0].ResetsAt.Equal(wantUntil) {
		t.Fatalf("resume at %v, want %v", flash.ts.notes[0].ResetsAt, wantUntil)
	}

	// Paused: no run, and still one note in the thread.
	if err := flash.agent.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if *flash.runs != 1 || len(flash.ts.notes) != 1 {
		t.Fatalf("ran or noted again while paused: runs %d notes %d", *flash.runs, len(flash.ts.notes))
	}
	if d := flash.agent.waitInterval(); d <= time.Hour {
		t.Fatalf("waits only %v while paused for hours", d)
	}

	// Another model on the same machine still runs.
	other := newAgyAgent(t, "claude-opus-4-6-thinking", limiter, &now, func(int) (RunResult, error) { return RunResult{Text: "done"}, nil })
	if err := other.agent.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if *other.runs != 1 {
		t.Fatal("the other model must keep working")
	}

	// A restart keeps the pause.
	restarted := NewAgyLimiter(state, nil, func() time.Time { return now })
	if _, ok := restarted.Paused("gemini-3.8-flash-high"); !ok {
		t.Fatal("the pause did not survive a restart")
	}

	// Past the reset (+1 minute) it runs again.
	now = wantUntil.Add(time.Second)
	if err := flash.agent.Tick(ctx); err != nil && !strings.Contains(err.Error(), "quota") {
		t.Fatal(err)
	}
	if *flash.runs != 2 {
		t.Fatalf("did not resume after the reset: runs %d", *flash.runs)
	}
}

func TestAgyQuotaWithoutResetRetriesIn30Minutes(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	limiter := NewAgyLimiter(filepath.Join(t.TempDir(), "q.json"), nil, func() time.Time { return now })
	s := newAgyAgent(t, "", limiter, &now, func(int) (RunResult, error) {
		return RunResult{}, &AgyRunError{Status: "ERROR", Reason: "RESOURCE_EXHAUSTED (code 429): Individual quota reached."}
	})
	if err := s.agent.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(s.ts.notes) != 1 || s.ts.notes[0].Window != "agy_quota_retry" || *s.ts.notes[0].Model != "default" {
		t.Fatalf("notes: %+v", s.ts.notes)
	}
	if !s.ts.notes[0].ResetsAt.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("retry at %v", s.ts.notes[0].ResetsAt)
	}
}

func TestAgyAnswerMentioningQuotaDoesNotPause(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	limiter := NewAgyLimiter("", nil, func() time.Time { return now })
	s := newAgyAgent(t, "gemini-3.8-flash-high", limiter, &now, func(int) (RunResult, error) {
		return RunResult{Text: "Individual quota reached — that is what the log said. RESOURCE_EXHAUSTED (code 429)."}, nil
	})
	if err := s.agent.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := limiter.Paused("gemini-3.8-flash-high"); ok || len(s.ts.notes) != 0 {
		t.Fatal("an answer about a quota paused the model")
	}
	// Nor does a failed run whose only quota mention is in the response text.
	err := &AgyRunError{Status: "ERROR", Response: "Individual quota reached"}
	if _, ok := asAgyQuota(err, now); ok {
		t.Fatal("the response field decided a pause")
	}
}

func TestAgyOtherFailureStillCountsAttempts(t *testing.T) {
	now := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	limiter := NewAgyLimiter("", nil, func() time.Time { return now })
	s := newAgyAgent(t, "gemini-3.8-flash-high", limiter, &now, func(int) (RunResult, error) {
		return RunResult{}, &AgyRunError{Status: "ERROR", Reason: "invalid model selection"}
	})
	_ = s.agent.Tick(context.Background())
	if s.agent.attempts["item-gemini-3.8-flash-high"] != 1 {
		t.Fatalf("attempts %d", s.agent.attempts["item-gemini-3.8-flash-high"])
	}
}
