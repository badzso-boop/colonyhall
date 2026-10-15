package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

// scriptedAdapter decides with a Go function instead of an AI client.
type scriptedAdapter struct {
	decide func(ctx context.Context, req guard.DecideRequest) (guard.Decision, error)
	work   func(ctx context.Context, req guard.WorkRequest) (string, error)
	calls  int
}

func (s *scriptedAdapter) Decide(ctx context.Context, req guard.DecideRequest) (guard.RunResult, error) {
	s.calls++
	_, refs := guard.BuildPrompt(req.Handle, req.Items, false)
	d, err := s.decide(ctx, req)
	return guard.RunResult{Decision: d, Refs: refs}, err
}

func (s *scriptedAdapter) Work(ctx context.Context, req guard.WorkRequest) (guard.RunResult, error) {
	text, err := s.work(ctx, req)
	return guard.RunResult{Text: text}, err
}

func (e *env) guardAgent(ad guard.Adapter, allowWork bool) *guard.Agent {
	cfg := guard.Config{Server: e.srv.URL, Agents: []guard.AgentConfig{{
		Client: "claude-code", TokenEnv: "UNUSED", Debounce: time.Millisecond, HaltCheck: 5 * time.Millisecond, AllowWork: allowWork,
	}}}
	if err := cfg.Validate(); err != nil {
		e.t.Fatal(err)
	}
	return &guard.Agent{
		Cfg:     cfg.Agents[0],
		Client:  &guard.Client{BaseURL: e.srv.URL, Token: server.DevAgentToken},
		Adapter: ad,
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func (e *env) tick(a *guard.Agent) {
	e.t.Helper()
	if err := e.app.Drain(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	if err := a.Tick(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) messages(channelID string) []message {
	var list struct{ Messages []message }
	e.do("GET", "/api/v1/channels/"+channelID+"/messages", server.DevAdminToken, nil, &list)
	return list.Messages
}

// TestGuardAnswersInOneRun: two items, one decision run; the guard posts the
// reply in the thread, adds the reaction and acknowledges both.
func TestGuardAnswersInOneRun(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent largest planet?"})
	thanks, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent thanks!"})

	ad := &scriptedAdapter{decide: func(_ context.Context, req guard.DecideRequest) (guard.Decision, error) {
		if len(req.Items) != 2 || req.Handle != "fake-agent" {
			t.Errorf("request: %+v", req)
		}
		return guard.Decision{Actions: []guard.Action{
			{Ref: "1", Kind: guard.ActReply, Text: "Jupiter"},
			{Ref: "2", Kind: guard.ActReact, Emoji: "👍"},
		}}, nil
	}}
	a := e.guardAgent(ad, false)
	e.tick(a)
	e.tick(a) // nothing left: no second run
	if ad.calls != 1 {
		t.Fatalf("decision runs: %d", ad.calls)
	}
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if last.Body != "Jupiter" || last.ThreadRootID != q.ID {
		t.Fatalf("reply: %+v", last)
	}
	for _, m := range msgs {
		if m.ID == thanks.ID && (len(m.Reactions) != 1 || m.Reactions[0].Emoji != "👍") {
			t.Fatalf("reaction: %+v", m.Reactions)
		}
	}
}

// TestGuardGivesThreadContext: a reply in a thread comes with the earlier
// messages, so the model needs no read call.
func TestGuardGivesThreadContext(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	root, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "Which DB do we use?"})
	e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent you know this", "thread_root_id": root.ID})
	ad := &scriptedAdapter{decide: func(_ context.Context, req guard.DecideRequest) (guard.Decision, error) {
		c := req.Items[0].Context
		if len(c) != 1 || c[0].Body != "Which DB do we use?" || c[0].Author != "norbi" {
			t.Errorf("context: %+v", c)
		}
		return guard.Decision{}, nil
	}}
	e.tick(e.guardAgent(ad, false))
	if ad.calls != 1 {
		t.Fatal("no run")
	}
}

// TestGuardStopsOnEmergency: an emergency stop during a run kills the run
// and nothing is posted; the item stays in the inbox for later.
func TestGuardStopsOnEmergency(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent long job"})
	started := make(chan struct{})
	ad := &scriptedAdapter{decide: func(ctx context.Context, _ guard.DecideRequest) (guard.Decision, error) {
		close(started)
		<-ctx.Done() // a client that would run for minutes
		return guard.Decision{Actions: []guard.Action{{Ref: "1", Kind: guard.ActReply, Text: "too late"}}}, ctx.Err()
	}}
	a := e.guardAgent(ad, false)
	go func() {
		<-started
		if s := e.do("POST", "/api/v1/admin/emergency-stop", server.DevAdminToken, map[string]any{"reason": "test"}, nil); s != http.StatusOK {
			t.Errorf("emergency stop: %d", s)
		}
	}()
	done := make(chan struct{})
	go func() { e.tick(a); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the run was not stopped")
	}
	for _, m := range e.messages(general) {
		if m.Body == "too late" {
			t.Fatal("posted after the stop")
		}
	}
	var in struct{ Total int }
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if in.Total != 1 {
		t.Fatalf("the item must stay pending, total %d", in.Total)
	}
}

// TestGuardWork: a work decision triggers a full run whose result is posted
// in the thread; with work disabled the guard declines instead.
func TestGuardWork(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent run the tests"})
	ad := &scriptedAdapter{
		decide: func(context.Context, guard.DecideRequest) (guard.Decision, error) {
			return guard.Decision{Actions: []guard.Action{{Ref: "1", Kind: guard.ActWork, Task: "go test ./..."}}}, nil
		},
		work: func(_ context.Context, req guard.WorkRequest) (string, error) {
			if !strings.Contains(req.Prompt, "go test ./...") {
				t.Errorf("work prompt: %s", req.Prompt)
			}
			return "All 42 tests pass.", nil
		},
	}
	e.tick(e.guardAgent(ad, true))
	msgs := e.messages(general)
	if last := msgs[len(msgs)-1]; last.Body != "All 42 tests pass." || last.ThreadRootID != q.ID {
		t.Fatalf("work result: %+v", last)
	}

	e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent deploy it"})
	e.tick(e.guardAgent(ad, false))
	msgs = e.messages(general)
	if last := msgs[len(msgs)-1]; !strings.Contains(last.Body, "only chat") {
		t.Fatalf("declined work: %+v", last)
	}
}
