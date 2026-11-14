// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Round 54: while the workspace storage is full the guard wakes no agent.

func waitWake(t *testing.T, a *Agent) {
	t.Helper()
	select {
	case <-a.wakeCh:
	case <-time.After(2 * time.Second):
		t.Fatal("the agent was not woken")
	}
}

// A full storage starts no run and leaves the items in the inbox (no ack,
// no attempt, no note); when there is room again the agent is woken and
// runs the waiting items once.
func TestStorageFullStartsNoRunAndResumesOnce(t *testing.T) {
	ok := func() (RunResult, error) { return RunResult{Text: "done"}, nil }
	b := newBusyAgent(t, nil, []Item{busyItem("i1", busyTh1), busyItem("i2", busyTh1)}, ok)
	b.agent.wakeCh = make(chan struct{}, 1)
	gate := NewStorageGate(slog.Default(), func(full bool) {
		if !full {
			b.agent.Wake()
		}
	})
	b.agent.Storage = gate
	ctx := context.Background()

	gate.Set(&HostStorage{State: "full", UsedBytes: 100, QuotaBytes: 100})
	for i := 0; i < 2; i++ {
		if err := b.agent.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	acked, notes := b.state()
	if b.runs.Load() != 0 || len(acked) != 0 || len(notes) != 0 || len(b.agent.attempts) != 0 {
		t.Fatalf("ran while the storage was full: runs %d acked %v notes %v attempts %v", b.runs.Load(), acked, notes, b.agent.attempts)
	}

	gate.Set(&HostStorage{State: "warning_95"})
	waitWake(t, b.agent)
	if err := b.agent.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	acked, _ = b.state()
	if b.runs.Load() != 1 || !slices.Equal(acked, []string{"i1", "i2"}) {
		t.Fatalf("after room: runs %d acked %v", b.runs.Load(), acked)
	}
	// The server drops acked items: nothing runs twice.
	b.ts.mu.Lock()
	b.ts.inboxItems = nil
	b.ts.mu.Unlock()
	if err := b.agent.Tick(ctx); err != nil || b.runs.Load() != 1 {
		t.Fatalf("ran again: %d %v", b.runs.Load(), err)
	}
}

// A run already going when the storage fills up finishes and is acked.
func TestStorageFullDoesNotStopARunningRun(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	run := func() (RunResult, error) {
		close(started)
		<-release
		return RunResult{Text: "done"}, nil
	}
	b := newBusyAgent(t, nil, []Item{busyItem("i1", busyTh1)}, run)
	gate := NewStorageGate(slog.Default(), nil)
	b.agent.Storage = gate
	errc := make(chan error, 1)
	go func() { errc <- b.agent.Tick(context.Background()) }()
	<-started
	gate.Set(&HostStorage{State: "full"})
	close(release)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if acked, _ := b.state(); !slices.Equal(acked, []string{"i1"}) {
		t.Fatalf("the running run was not finished: acked %v", acked)
	}
}

// An older server sends no storage: nothing changes, and a nil gate (an
// agent outside a supervisor) never stops runs either.
func TestStorageMissingFromAnOlderServer(t *testing.T) {
	var cfg HostConfig
	if err := json.Unmarshal([]byte(`{"version":"v1","agents":[],"previews":[]}`), &cfg); err != nil {
		t.Fatal(err)
	}
	gate := NewStorageGate(slog.Default(), nil)
	gate.Set(cfg.Storage)
	if cfg.Storage != nil || gate.Full() {
		t.Fatal("an older server's config paused the agents")
	}
	var none *StorageGate
	none.Set(&HostStorage{State: "full"})
	if none.Full() {
		t.Fatal("a nil gate is full")
	}
	ok := func() (RunResult, error) { return RunResult{Text: "done"}, nil }
	b := newBusyAgent(t, nil, []Item{busyItem("i1", busyTh1)}, ok)
	if err := b.agent.Tick(context.Background()); err != nil || b.runs.Load() != 1 {
		t.Fatalf("without a gate: runs %d %v", b.runs.Load(), err)
	}
}

// The supervisor takes the state from the host config, tells the server in
// its report that the agents are paused, wakes every agent when there is
// room, and fetches the config at once when an agent's WebSocket announces
// a change.
func TestSupervisorStoragePauseReportAndResume(t *testing.T) {
	var mu sync.Mutex
	var reports []HostReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/host/report") {
			var rep HostReport
			_ = json.NewDecoder(r.Body).Decode(&rep)
			mu.Lock()
			reports = append(reports, rep)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	s := &Supervisor{Cfg: Config{}, Log: slog.Default(), host: &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		agents: map[string]*managed{}}
	woken := make(chan bool, 4)
	s.storage = NewStorageGate(s.Log, func(full bool) {
		s.report(context.Background())
		woken <- full
	})
	lastReport := func() HostReport {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			n := len(reports)
			var r HostReport
			if n > 0 {
				r = reports[n-1]
			}
			mu.Unlock()
			if n > 0 {
				return r
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("no report")
		return HostReport{}
	}

	s.reconcile(context.Background(), HostConfig{Version: "v1", Storage: &HostStorage{State: "full", UsedBytes: 10, QuotaBytes: 10}})
	if full := <-woken; !full || !s.storage.Full() {
		t.Fatal("the config's full state did not pause the machine")
	}
	if !lastReport().StoragePaused {
		t.Fatal("the report does not say the agents are paused")
	}
	mu.Lock()
	reports = nil
	mu.Unlock()
	s.reconcile(context.Background(), HostConfig{Version: "v2", Storage: &HostStorage{State: "ok"}})
	if full := <-woken; full || s.storage.Full() {
		t.Fatal("room did not resume the machine")
	}
	if lastReport().StoragePaused {
		t.Fatal("the report still says paused")
	}

	// An agent's WebSocket event asks for the config now, without blocking.
	a := &Agent{OnStorageEvent: s.SyncNow}
	a.onChannelEvent("workspace.storage_state_changed", json.RawMessage(`{"state":"full"}`))
	a.onChannelEvent("workspace.storage_state_changed", json.RawMessage(`{"state":"full"}`))
	select {
	case <-s.syncSignal():
	default:
		t.Fatal("the storage event did not ask for the config")
	}
}
