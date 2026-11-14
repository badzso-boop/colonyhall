// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// An agent that disappears from the host config was deleted (archived):
// its terminals are killed with that reason; a restart keeps them.
func TestStopKillsTerminalsWithReason(t *testing.T) {
	tm, tmp := testTerminalManager(t)
	s := &Supervisor{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), terminals: tm, agents: map[string]*managed{}}
	add := func(id, handle string) {
		_, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		close(done)
		s.agents[id] = &managed{cancel: cancel, done: done, conf: HostAgent{AgentID: id, Handle: handle}}
	}
	for _, h := range []string{"kept", "gone"} {
		if _, err := tm.Start("id-"+h, h, "job", "sleep 60", tmp, "ch-1", "", nil); err != nil {
			t.Fatal(err)
		}
		add("id-"+h, h)
	}
	s.stop("id-kept", "")
	s.stop("id-gone", KilledArchived)
	if m, err := tm.Get("kept", "job"); err != nil || m.FinishedAt != nil {
		t.Fatalf("a restart killed the terminal: %+v %v", m, err)
	}
	m, err := tm.Get("gone", "job")
	if err != nil || m.Status() != "killed" || m.KilledBy != "archived" {
		t.Fatalf("archived agent's terminal: %+v %v", m, err)
	}
}

func TestSessionLimitChangesLive(t *testing.T) {
	a := &Agent{Cfg: AgentConfig{MaxSessionTokens: 150000}}
	if a.maxSessionTokens() != 150000 {
		t.Fatal("config value")
	}
	a.SetSessionLimit(0)
	if a.maxSessionTokens() != 0 {
		t.Fatal("no limit")
	}
	a.SetSessionLimit(300000)
	if a.maxSessionTokens() != 300000 {
		t.Fatal("new limit")
	}
}
