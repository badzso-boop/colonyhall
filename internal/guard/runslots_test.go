// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	busyCh  = "11111111-1111-1111-1111-111111111111"
	busyTh1 = "22222222-2222-2222-2222-222222222222"
	busyTh2 = "33333333-3333-3333-3333-333333333333"
)

type busyAgent struct {
	ts    *ratelimitTestServer
	agent *Agent
	runs  *atomic.Int32
}

func newBusyAgent(t *testing.T, slots *RunSlots, items []Item, run func() (RunResult, error)) busyAgent {
	t.Helper()
	ts := &ratelimitTestServer{inboxItems: items}
	srv := httptest.NewServer(ts.handler())
	t.Cleanup(srv.Close)
	runs := &atomic.Int32{}
	adapter := &scriptedAdapter{directFn: func(context.Context, DirectRequest) (RunResult, error) {
		runs.Add(1)
		return run()
	}}
	agent := &Agent{
		Cfg:      AgentConfig{Client: "antigravity", Mode: "direct", Batch: 10, Model: "m", WorkModel: "m", Poll: time.Hour, FallbackPoll: time.Hour},
		Client:   &Client{BaseURL: srv.URL, HTTP: srv.Client()},
		Adapter:  adapter,
		StateDir: t.TempDir(),
		Log:      slog.Default(),
		RunSlots: slots,
	}
	return busyAgent{ts: ts, agent: agent, runs: runs}
}

func busyItem(id, thread string) Item {
	return Item{ID: id, Kind: "message", ItemID: "msg-" + id, ChannelID: busyCh, ThreadRootID: thread, Author: "alice", AuthorKind: "human", Body: "run task"}
}

func (b busyAgent) state() (acked []string, notes []string) {
	b.ts.mu.Lock()
	defer b.ts.mu.Unlock()
	for _, n := range b.ts.notes {
		th := ""
		if n.ThreadRootId != nil {
			th = n.ThreadRootId.String()
		}
		notes = append(notes, string(n.Window)+"@"+th)
	}
	return append([]string(nil), b.ts.ackedIDs...), notes
}

// Without a free slot the items stay pending, no attempt counts, and each
// thread gets one note; a freed slot wakes the agent and it runs.
func TestRunSlotsKeepItemsPendingWithOneNotePerThread(t *testing.T) {
	slots := NewRunSlots(1)
	if !slots.TryAcquire(nil) { // another agent's run holds the only slot
		t.Fatal("empty limit refused a run")
	}
	ok := func() (RunResult, error) { return RunResult{Text: "done"}, nil }
	b := newBusyAgent(t, slots, []Item{busyItem("i1", busyTh1), busyItem("i2", busyTh1), busyItem("i3", busyTh2)}, ok)
	b.agent.wakeCh = make(chan struct{}, 1)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := b.agent.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	acked, notes := b.state()
	if b.runs.Load() != 0 || len(acked) != 0 || len(b.agent.attempts) != 0 {
		t.Fatalf("ran while the machine was full: runs %d acked %v attempts %v", b.runs.Load(), acked, b.agent.attempts)
	}
	if len(notes) != 2 || notes[0] != "machine_busy@"+busyTh1 || notes[1] != "machine_busy@"+busyTh2 {
		t.Fatalf("want one busy note per thread, got %v", notes)
	}

	slots.Release()
	select {
	case <-b.agent.wakeCh:
	default:
		t.Fatal("a freed slot did not wake the waiting agent")
	}
	if err := b.agent.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	acked, _ = b.state()
	if b.runs.Load() == 0 || len(acked) != 3 {
		t.Fatalf("did not run after the slot freed: runs %d acked %v", b.runs.Load(), acked)
	}
	if slots.InUse() != 0 {
		t.Fatalf("the run kept its slot: %d in use", slots.InUse())
	}

	// A later wait is a new one: the thread hears about it again.
	slots.TryAcquire(nil)
	b.ts.mu.Lock()
	b.ts.inboxItems, b.ts.ackedIDs = []Item{busyItem("i4", busyTh1)}, nil
	b.ts.mu.Unlock()
	_ = b.agent.Tick(ctx)
	if _, notes = b.state(); len(notes) != 3 {
		t.Fatalf("notes after a new wait: %v", notes)
	}
}

// Three agents, two slots: two run at once, the third waits.
func TestRunSlotsLimitConcurrentRuns(t *testing.T) {
	slots := NewRunSlots(2)
	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	block := func() (RunResult, error) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		return RunResult{Text: "done"}, nil
	}
	var agents []busyAgent
	for i, th := range []string{busyTh1, busyTh2, busyTh1} {
		agents = append(agents, newBusyAgent(t, slots, []Item{busyItem("i"+string(rune('a'+i)), th)}, block))
	}
	ctx := context.Background()
	var wg sync.WaitGroup
	done := make(chan int, len(agents))
	for i, b := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.agent.Tick(ctx)
			done <- i
		}()
	}
	// The one without a slot returns at once, with its note.
	var waiting int
	select {
	case waiting = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("no agent was turned away")
	}
	deadline := time.Now().Add(5 * time.Second)
	for inFlight.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := inFlight.Load(); got != 2 {
		t.Fatalf("%d runs in flight, want 2", got)
	}
	if _, notes := agents[waiting].state(); agents[waiting].runs.Load() != 0 || len(notes) != 1 {
		t.Fatalf("the waiting agent: runs %d notes %v", agents[waiting].runs.Load(), notes)
	}
	close(release)
	wg.Wait()
	if peak.Load() != 2 {
		t.Fatalf("peak %d concurrent runs, limit 2", peak.Load())
	}
	if err := agents[waiting].agent.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if agents[waiting].runs.Load() != 1 {
		t.Fatal("the waiting agent did not run once slots were free")
	}
}

// End to end in the agent loop: the queued run starts by itself when the
// other run ends, without waiting for a poll.
func TestRunSlotsQueuedRunStartsWhenASlotFrees(t *testing.T) {
	slots := NewRunSlots(1)
	slots.TryAcquire(nil)
	b := newBusyAgent(t, slots, []Item{busyItem("i1", busyTh1)}, func() (RunResult, error) { return RunResult{Text: "done"}, nil })
	b.agent.DisableWS = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = b.agent.Run(ctx) }()

	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	waitFor("the loop did not start", func() bool {
		b.agent.wakeMu.Lock()
		defer b.agent.wakeMu.Unlock()
		return b.agent.wakeCh != nil
	})
	b.agent.Wake() // a new message arrives
	waitFor("no busy note", func() bool { _, n := b.state(); return len(n) == 1 })
	if b.runs.Load() != 0 {
		t.Fatal("ran without a slot")
	}
	slots.Release()
	waitFor("the queued run did not start when the slot freed", func() bool { return b.runs.Load() == 1 })
}

func TestRunSlotsNilMeansNoLimit(t *testing.T) {
	r := NewRunSlots(0)
	for i := 0; i < 100; i++ {
		if !r.TryAcquire(nil) {
			t.Fatal("refused without a limit")
		}
	}
	r.Release()
}
