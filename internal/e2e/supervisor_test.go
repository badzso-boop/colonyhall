package e2e

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"colonyhall/internal/guard"
	"colonyhall/internal/server"
)

// eventually polls cond until it holds or the deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestSupervisorHostMode: a paired machine runs the agent the admin
// assigned, with only the grants that pass the machine's own checks, and
// stops it when the assignment ends.
func TestSupervisorHostMode(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = e.app.Run(ctx) }() // delivery and audit consumers

	base := t.TempDir()
	root := filepath.Join(base, "projects")
	app, docs := filepath.Join(root, "app"), filepath.Join(root, "docs")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{app, docs, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	// Pair the machine the way `colony guard enroll` does.
	var pc struct{ Code string }
	e.do("POST", "/api/v1/hosts/pairing-codes", server.DevAdminToken, nil, &pc)
	tokenFile := filepath.Join(base, "host.token")
	if err := guard.Enroll(ctx, e.srv.URL, pc.Code, "test-box", tokenFile); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(tokenFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("token file: %v %v", fi, err)
	}
	var hosts struct{ Hosts []struct{ ID string } }
	e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &hosts)
	hostID := hosts.Hosts[0].ID

	agent := e.agentID()
	e.configure(agent, map[string]any{"host_id": hostID, "client": "claude-code", "model": "haiku", "allow_work": true, "mode": "compact"},
		map[string]any{"path": app, "mode": "write"}, map[string]any{"path": docs, "mode": "read"})
	// The server refuses a grant outside the roots at creation; one made
	// before the machine's guard.yaml narrowed its roots still reaches the
	// guard, which refuses it on its own.
	e.grantDirectly(agent, hostID, outside, "write")

	var mu sync.Mutex
	var works []guard.WorkRequest
	ad := &scriptedAdapter{
		decide: func(context.Context, guard.DecideRequest) (guard.Decision, error) {
			return guard.Decision{Actions: []guard.Action{{Ref: "1", Kind: guard.ActWork, Task: "list files"}}}, nil
		},
		work: func(_ context.Context, req guard.WorkRequest) (string, error) {
			mu.Lock()
			works = append(works, req)
			mu.Unlock()
			return "done in " + filepath.Base(req.Dir), nil
		},
	}
	off := false
	sup := &guard.Supervisor{
		Cfg: guard.Config{Server: e.srv.URL, HostTokenFile: tokenFile, Roots: []string{root}, SandboxDir: filepath.Join(base, "sandbox"),
			RequireSandbox: &off, ConfigPoll: 20 * time.Millisecond,
			Defaults: guard.AgentConfig{Poll: 20 * time.Millisecond, Debounce: time.Millisecond, HaltCheck: 10 * time.Millisecond}},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(guard.AgentConfig) guard.Adapter { return ad },
	}
	if err := sup.Cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = sup.RunHost(ctx) }()

	// The grant outside the machine's roots is refused and reported.
	eventually(t, "the refused grant in the report", func() bool {
		var list struct {
			Hosts []struct {
				Report struct {
					Roots    []string
					Rejected []struct{ Path, Reason string }
				}
			}
		}
		e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, &list)
		r := list.Hosts[0].Report
		return len(r.Rejected) == 1 && r.Rejected[0].Path == outside && strings.Contains(r.Rejected[0].Reason, "outside") && len(r.Roots) == 1
	})

	general := e.generalID()
	q, _ := e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent list the files"})
	eventually(t, "the work result", func() bool {
		msgs := e.messages(general)
		last := msgs[len(msgs)-1]
		return last.ThreadRootID == q.ID && last.Body == "done in app"
	})
	mu.Lock()
	w := works[0]
	mu.Unlock()
	realDocs, _ := filepath.EvalSymlinks(docs)
	if len(w.AddDirs) != 1 || w.AddDirs[0] != realDocs {
		t.Fatalf("work dirs: dir %s, add %v", w.Dir, w.AddDirs)
	}

	// Deleting (archiving) the agent stops it: a new mention is not answered.
	if s := e.do("DELETE", "/api/v1/agents/"+agent, server.DevAdminToken, nil, nil); s != http.StatusNoContent {
		t.Fatalf("archive: %d", s)
	}
	time.Sleep(200 * time.Millisecond) // a few config polls
	n := len(e.messages(general))
	e.send(server.DevAdminToken, general, map[string]any{"body": "@fake-agent are you there?"})
	time.Sleep(300 * time.Millisecond)
	if got := len(e.messages(general)); got != n+1 {
		t.Fatalf("an unassigned agent answered: %d messages, want %d", got, n+1)
	}
	if s := e.do("GET", "/api/v1/hosts", server.DevAdminToken, nil, nil); s != http.StatusOK {
		t.Fatal(s)
	}
}
