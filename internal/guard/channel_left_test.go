package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// channelLeftEnv runs one direct run that blocks until it is cancelled, and
// records every note the guard posts.
type channelLeftEnv struct {
	agent   *Agent
	started chan struct{}
	done    chan error
	mu      sync.Mutex
	notes   []string
}

func newChannelLeftEnv(t *testing.T, agentID, channelID string) *channelLeftEnv {
	t.Helper()
	env := &channelLeftEnv{started: make(chan struct{}), done: make(chan error, 1)}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "handle": "bot", "state": "active"})
		case strings.HasSuffix(r.URL.Path, "/actors"):
			_ = json.NewEncoder(w).Encode(map[string]any{"actors": []map[string]any{}})
		case strings.HasSuffix(r.URL.Path, "/messages"), strings.HasSuffix(r.URL.Path, "/agent/retract-note"):
			env.mu.Lock()
			env.notes = append(env.notes, r.URL.Path)
			env.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(ts.Close)
	ad := &directTestAdapter{}
	ad.impl = func(ctx context.Context, req DirectRequest) (RunResult, error) {
		close(env.started)
		<-ctx.Done()
		return RunResult{}, ctx.Err()
	}
	env.agent = &Agent{
		Cfg:      AgentConfig{Client: "claude-code", IdleTimeout: 10 * time.Minute, MaxRun: time.Hour, HaltCheck: time.Hour},
		Client:   &Client{BaseURL: ts.URL, Token: "tok", HTTP: ts.Client()},
		Adapter:  ad,
		StateDir: t.TempDir(),
		RootDir:  t.TempDir(),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
		me:       Me{ID: agentID, Handle: "bot"},
	}
	items := []Item{{ID: "it-1", AuthorKind: "human", ItemID: "m1", ChannelID: channelID, ThreadRootID: "m1", Body: "task"}}
	go func() { env.done <- env.agent.handleDirect(context.Background(), items) }()
	<-env.started
	return env
}

func (env *channelLeftEnv) waitStopped(t *testing.T) {
	t.Helper()
	select {
	case err := <-env.done:
		if err != nil {
			t.Fatalf("a stopped run must end without error, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the run did not stop")
	}
	env.mu.Lock()
	defer env.mu.Unlock()
	if len(env.notes) != 0 {
		t.Fatalf("no note may be posted when the channel is left, got %v", env.notes)
	}
}

func (env *channelLeftEnv) stillRunning(t *testing.T) {
	t.Helper()
	select {
	case err := <-env.done:
		t.Fatalf("the run must keep going, it ended with %v", err)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestRemovalStopsOnlyTheMatchingRun(t *testing.T) {
	env := newChannelLeftEnv(t, "agent-1", "c1")

	// Another agent removed, or this agent removed elsewhere: keep going.
	env.agent.onChannelEvent("channel.member_removed", json.RawMessage(`{"channel_id":"c1","actor_id":"agent-2"}`))
	env.agent.onChannelEvent("channel.member_removed", json.RawMessage(`{"channel_id":"c2","actor_id":"agent-1"}`))
	env.stillRunning(t)

	env.agent.onChannelEvent("channel.member_removed", json.RawMessage(`{"channel_id":"c1","actor_id":"agent-1"}`))
	env.waitStopped(t)
}

func TestArchiveStopsEveryRunInTheChannel(t *testing.T) {
	a := newChannelLeftEnv(t, "agent-1", "c1")
	b := newChannelLeftEnv(t, "agent-2", "c1")
	other := newChannelLeftEnv(t, "agent-3", "c2")

	ev := json.RawMessage(`{"channel_id":"c1","name":"ops"}`)
	for _, env := range []*channelLeftEnv{a, b, other} {
		env.agent.onChannelEvent("channel.archived", ev)
	}
	a.waitStopped(t)
	b.waitStopped(t)
	other.stillRunning(t)
	other.agent.OnChannelLeft("c2", "archived")
	other.waitStopped(t)
}

func TestDecideSkipsItemsOfALeftChannel(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "agent-1", "handle": "bot", "state": "active"})
		case strings.HasSuffix(r.URL.Path, "/messages") && r.Method == http.MethodPost:
			mu.Lock()
			sent = append(sent, r.URL.Path)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":{"id":"r1"}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer ts.Close()
	var agent *Agent
	ad := &directTestAdapter{decideImpl: func(ctx context.Context, req DecideRequest) (RunResult, error) {
		// The agent is removed from c1 while it decides.
		agent.onChannelEvent("channel.member_removed", json.RawMessage(`{"channel_id":"c1","actor_id":"agent-1"}`))
		return RunResult{Decision: Decision{Actions: []Action{
			{Ref: "1", Kind: ActReply, Text: "answer in c1"},
			{Ref: "2", Kind: ActReply, Text: "answer in c2"},
		}}}, nil
	}}
	agent = &Agent{
		Cfg:      AgentConfig{Client: "claude-code", DecideTimeout: time.Minute, IdleTimeout: time.Minute, MaxRun: time.Hour, HaltCheck: time.Hour},
		Client:   &Client{BaseURL: ts.URL, Token: "tok", HTTP: ts.Client()},
		Adapter:  ad,
		StateDir: t.TempDir(),
		RootDir:  t.TempDir(),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		attempts: map[string]int{},
		me:       Me{ID: "agent-1", Handle: "bot"},
	}
	items := []Item{
		{ID: "it-1", AuthorKind: "human", ItemID: "m1", ChannelID: "c1", Body: "q1", Mentioned: true},
		{ID: "it-2", AuthorKind: "human", ItemID: "m2", ChannelID: "c2", Body: "q2", Mentioned: true},
	}
	if err := agent.handle(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || !strings.Contains(sent[0], "c2") {
		t.Fatalf("only the c2 answer may be posted, got %v", sent)
	}
}
