package e2e

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

func TestSupervisorDirectMode(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	// Pair machine
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "test-box", tokenFile); err != nil {
		t.Fatal(err)
	}
	var hosts struct{ Hosts []struct{ ID string } }
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	hostID := hosts.Hosts[0].ID

	agent := e.agentID()
	// Assign in direct mode
	e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{
			"host_id":         hostID,
			"client":          "claude-code",
			"mode":            "direct",
			"root_dir":        root,
			"sandbox":         false,
			"permission_mode": "auto",
		}, nil)

	var mu sync.Mutex
	var directReqs []guard.DirectRequest
	var waitCh chan struct{}
	var startedCh chan struct{}

	ad := &scriptedAdapter{
		direct: func(ctx context.Context, req guard.DirectRequest) (guard.RunResult, error) {
			mu.Lock()
			directReqs = append(directReqs, req)
			s := startedCh
			w := waitCh
			mu.Unlock()
			if s != nil {
				close(s)
			}
			if w != nil {
				select {
				case <-ctx.Done():
					return guard.RunResult{Text: "aborted"}, ctx.Err()
				case <-w:
				}
			}
			return guard.RunResult{Text: "reply for " + req.SessionID}, nil
		},
	}

	off := false
	sup := &guard.Supervisor{
		Cfg: guard.Config{
			Server:           e.srv.URL,
			HostTokenFile:    tokenFile,
			Roots:            []string{root},
			SandboxDir:       filepath.Join(base, "sandbox"),
			RequireSandbox:   &off,
			AllowUnsandboxed: true,
			ConfigPoll:       20 * time.Millisecond,
			Defaults: guard.AgentConfig{
				Poll:      20 * time.Millisecond,
				Debounce:  time.Millisecond,
				HaltCheck: 10 * time.Millisecond,
			},
		},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(guard.AgentConfig) guard.Adapter { return ad },
	}
	if err := sup.Cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = sup.RunHost(ctx) }()

	general := e.generalID()

	// 1. Thread 1, first message -> run uses --session-id (Resume: false)
	q1, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent question 1"})
	eventually(t, "reply for question 1", func() bool {
		msgs := e.messages(general)
		last := msgs[len(msgs)-1]
		return last.ThreadRootID == q1.ID && last.AuthorID == agent
	})

	mu.Lock()
	if len(directReqs) != 1 {
		t.Fatalf("expected 1 direct req, got %d", len(directReqs))
	}
	req1 := directReqs[0]
	mu.Unlock()

	if req1.Resume {
		t.Fatal("expected Resume=false on first run in thread 1")
	}
	if req1.SessionID == "" {
		t.Fatal("expected non-empty SessionID")
	}
	if req1.Dir != root {
		t.Fatalf("expected Dir=%s, got %s", root, req1.Dir)
	}

	// 2. Thread 1, second message -> run uses same session ID with Resume: true
	_, _ = e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent follow up 1", "thread_root_id": q1.ID})
	eventually(t, "reply for follow up 1", func() bool {
		msgs := e.messages(general)
		last := msgs[len(msgs)-1]
		return last.ThreadRootID == q1.ID && last.AuthorID == agent && last.Body == "reply for "+req1.SessionID
	})

	mu.Lock()
	if len(directReqs) != 2 {
		t.Fatalf("expected 2 direct reqs, got %d", len(directReqs))
	}
	req2 := directReqs[1]
	mu.Unlock()

	if !req2.Resume {
		t.Fatal("expected Resume=true on second run in thread 1")
	}
	if req2.SessionID != req1.SessionID {
		t.Fatalf("expected same SessionID %s, got %s", req1.SessionID, req2.SessionID)
	}

	// 3. Thread 2, new message -> run uses different session ID with Resume: false
	q3, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent new thread message"})
	eventually(t, "reply for new thread message", func() bool {
		msgs := e.messages(general)
		last := msgs[len(msgs)-1]
		return last.ThreadRootID == q3.ID && last.AuthorID == agent
	})

	mu.Lock()
	if len(directReqs) != 3 {
		t.Fatalf("expected 3 direct reqs, got %d", len(directReqs))
	}
	req3 := directReqs[2]
	mu.Unlock()

	if req3.Resume {
		t.Fatal("expected Resume=false for new thread")
	}
	if req3.SessionID == req1.SessionID {
		t.Fatalf("expected different SessionID for new thread, got %s", req3.SessionID)
	}

	// 4. Verify items were acknowledged
	eventually(t, "inbox cleared", func() bool {
		var in struct{ Total int }
		e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
		return in.Total == 0
	})

	// 5. Emergency stop during a run kills the run and nothing is posted
	mu.Lock()
	startedCh = make(chan struct{})
	waitCh = make(chan struct{})
	mu.Unlock()

	q4, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent do slow task"})

	// Wait until direct handler is called
	select {
	case <-startedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("adapter was not called for q4")
	}

	// Trigger emergency stop while run is in progress
	if s := e.do("POST", "/api/v1/admin/emergency-stop", server.DevAdminToken, map[string]any{"reason": "stop now"}, nil); s != http.StatusOK {
		t.Fatalf("emergency stop status: %d", s)
	}

	// Give time for supervisor to notice halt and abort the run
	time.Sleep(200 * time.Millisecond)

	// Verify nothing was posted in response to q4
	for _, m := range e.messages(general) {
		if m.ThreadRootID == q4.ID && m.AuthorID == agent {
			t.Fatal("posted a reply after emergency stop")
		}
	}

	// Verify q4 remains unacknowledged in inbox
	var in struct{ Total int }
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if in.Total != 1 {
		t.Fatalf("expected 1 pending item in inbox after emergency stop, got %d", in.Total)
	}
}

func TestSupervisorAllowUnsandboxedE2E(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }()

	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	// Pair machine
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "test-box", tokenFile); err != nil {
		t.Fatal(err)
	}
	var hosts struct{ Hosts []struct{ ID string } }
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	hostID := hosts.Hosts[0].ID

	agent := e.agentID()
	// Assign in direct mode with sandbox: false
	e.do("PUT", "/api/v1/agents/"+agent+"/host", server.DevAdminToken,
		map[string]any{
			"host_id":         hostID,
			"client":          "claude-code",
			"mode":            "direct",
			"root_dir":        root,
			"sandbox":         false,
			"permission_mode": "auto",
		}, nil)

	called := false
	ad := &scriptedAdapter{
		direct: func(ctx context.Context, req guard.DirectRequest) (guard.RunResult, error) {
			called = true
			return guard.RunResult{Text: "reply"}, nil
		},
	}

	off := false
	// Start supervisor with default AllowUnsandboxed: false
	sup := &guard.Supervisor{
		Cfg: guard.Config{
			Server:           e.srv.URL,
			HostTokenFile:    tokenFile,
			Roots:            []string{root},
			SandboxDir:       filepath.Join(base, "sandbox"),
			RequireSandbox:   &off,
			AllowUnsandboxed: false,
			ConfigPoll:       20 * time.Millisecond,
			Defaults: guard.AgentConfig{
				Poll:      20 * time.Millisecond,
				Debounce:  time.Millisecond,
				HaltCheck: 10 * time.Millisecond,
			},
		},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(guard.AgentConfig) guard.Adapter { return ad },
	}
	if err := sup.Cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = sup.RunHost(ctx) }()

	// Verify host report has allow_unsandboxed: false and agent is rejected
	eventually(t, "unsandboxed agent refused and reported", func() bool {
		var list struct {
			Hosts []struct {
				Report struct {
					AllowUnsandboxed bool `json:"allow_unsandboxed"`
					Rejected         []struct {
						AgentID string `json:"agent_id"`
						Path    string `json:"path"`
						Reason  string `json:"reason"`
					} `json:"rejected"`
				} `json:"report"`
			} `json:"hosts"`
		}
		e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &list)
		if len(list.Hosts) == 0 {
			return false
		}
		rep := list.Hosts[0].Report
		if rep.AllowUnsandboxed {
			return false
		}
		for _, r := range rep.Rejected {
			if r.AgentID == agent && strings.Contains(r.Reason, "allow_unsandboxed") {
				return true
			}
		}
		return false
	})

	// Message is not answered because agent loop was refused
	general := e.generalID()
	e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent hello"})
	time.Sleep(100 * time.Millisecond)
	if called {
		t.Fatal("agent should not have been called when allow_unsandboxed is false")
	}
}

// TestDirectModeNeverSilent: a run that ends without an answer (here the
// client refused a write, as headless agy does) still tells the thread why;
// before, the item was acked and the sender saw nothing.
func TestDirectModeNeverSilent(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent create notes.txt"})
	ad := &scriptedAdapter{direct: func(context.Context, guard.DirectRequest) (guard.RunResult, error) {
		return guard.RunResult{Text: "", Denied: []string{"write_file"}}, nil
	}}
	a := e.guardAgent(ad, false)
	dir := t.TempDir()
	a.Mode, a.RootDir, a.StateDir = "direct", dir, dir
	e.tick(a)
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if last.ThreadRootID != q.ID || !strings.HasPrefix(last.Body, "⚠️") || strings.Contains(last.Body, "write_file") {
		t.Fatalf("the thread gets a short note, not the details: %+v", last)
	}
	if d := e.errorDetail(last.Body); !strings.Contains(d, "write_file") {
		t.Fatalf("the details are for humans: %q", d)
	}
}

// TestGiveUpIsAnnounced: after repeated failures (e.g. the provider is
// down) the guard drops the item but tells the thread; before, the item
// vanished without a word.
func TestGiveUpIsAnnounced(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent second question"})
	ad := &scriptedAdapter{direct: func(context.Context, guard.DirectRequest) (guard.RunResult, error) {
		return guard.RunResult{}, errors.New("agy: Eligibility check failed: UNAVAILABLE (code 503); token col_abcdefghijklmnop")
	}}
	a := e.guardAgent(ad, false)
	dir := t.TempDir()
	a.Mode, a.RootDir, a.StateDir = "direct", dir, dir
	for range 3 {
		if err := e.app.Drain(context.Background()); err != nil {
			t.Fatal(err)
		}
		_ = a.Tick(context.Background())
	}
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if last.ThreadRootID != q.ID || !strings.HasPrefix(last.Body, "⚠️") || strings.Contains(last.Body, "503") {
		t.Fatalf("the give-up is announced without details: %+v", last)
	}
	d := e.errorDetail(last.Body)
	if !strings.Contains(d, "503") || strings.Contains(d, "abcdefghijklmnop") {
		t.Fatalf("details for humans, secrets redacted: %q", d)
	}
	var in struct{ Total int }
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if in.Total != 0 {
		t.Fatalf("the item is acked after the announcement, total %d", in.Total)
	}
}

var errorRef = regexp.MustCompile(`\[error:([0-9a-f-]{36})\]`)

// errorDetail follows a chat note's [error:<id>] reference: humans get the
// details, agents get 403.
func (e *env) errorDetail(body string) string {
	e.t.Helper()
	m := errorRef.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no error reference in %q", body)
	}
	if s := e.do("GET", "/api/v1/errors/"+m[1], server.DevAgentToken, nil, nil); s != http.StatusForbidden {
		e.t.Fatalf("an agent reads error details: %d", s)
	}
	var out struct{ Detail string }
	if s := e.do("GET", "/api/v1/errors/"+m[1], server.DevAdminToken, nil, &out); s != http.StatusOK {
		e.t.Fatalf("a human reads error details: %d", s)
	}
	return out.Detail
}

// TestFullAccessAgentTakesTasksFromHumansOnly: an agent cannot give a
// full-access (direct mode) agent a task; the run never starts.
func TestFullAccessAgentTakesTasksFromHumansOnly(t *testing.T) {
	e := newEnv(t)
	general := e.generalID()
	_, otherToken := e.createActor("agent", "helper-bot", "member")
	agentID := e.agentID()
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": agentID}, nil)
	var me struct{ ID string }
	e.do("GET", "/api/v1/me", otherToken, nil, &me)
	e.do("POST", "/api/v1/channels/"+general+"/members", server.DevAdminToken, map[string]any{"actor_id": me.ID}, nil)
	q, s := e.send(otherToken, general, map[string]any{"body": "@fake-agent delete the build folder"})
	if s != http.StatusCreated {
		t.Fatalf("agent message: %d", s)
	}
	calls := 0
	ad := &scriptedAdapter{direct: func(context.Context, guard.DirectRequest) (guard.RunResult, error) {
		calls++
		return guard.RunResult{Text: "done"}, nil
	}}
	a := e.guardAgent(ad, false)
	dir := t.TempDir()
	a.Mode, a.RootDir, a.StateDir = "direct", dir, dir
	e.tick(a)
	if calls != 0 {
		t.Fatal("a task from an agent started a full-access run")
	}
	msgs := e.messages(general)
	last := msgs[len(msgs)-1]
	if last.ThreadRootID != q.ID || !strings.Contains(last.Body, "only take tasks from humans") {
		t.Fatalf("the sender is told: %+v", last)
	}
	var in struct{ Total int }
	e.do("GET", "/api/v1/agent/inbox", server.DevAgentToken, nil, &in)
	if in.Total != 0 {
		t.Fatalf("the refused item is acked, total %d", in.Total)
	}

	// A human asking the same runs it.
	e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent delete the build folder"})
	e.tick(a)
	if calls != 1 {
		t.Fatalf("a human's task must run: %d calls", calls)
	}
}
