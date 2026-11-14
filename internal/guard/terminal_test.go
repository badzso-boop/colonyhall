// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

package guard

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func skipIfNoTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed; skipping test")
	}
}

func testTerminalManager(t *testing.T) (*TerminalManager, string) {
	t.Helper()
	skipIfNoTmux(t)
	tmp := t.TempDir()
	roots := []string{tmp}
	tm := NewTerminalManager(filepath.Join(tmp, "state"), roots, tmp)
	// Unique tmux socket per test to prevent collision
	sock := fmt.Sprintf("colonyhall-test-%d", time.Now().UnixNano())
	tm.SetSocketName(sock)
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", sock, "kill-server").Run()
	})
	return tm, tmp
}

func TestTerminalNameRules(t *testing.T) {
	valid := []string{"build", "a", "ros-build-42", "node123", "a-b-c"}
	for _, n := range valid {
		if !termNameRe.MatchString(n) {
			t.Errorf("expected valid: %s", n)
		}
	}

	invalid := []string{"", "-bad", "Bad", "build_1", "with space", strings.Repeat("a", 33), "UPPER"}
	for _, n := range invalid {
		if termNameRe.MatchString(n) {
			t.Errorf("expected invalid: %s", n)
		}
	}
}

func TestTerminalCwdOutsideRootRefused(t *testing.T) {
	tm, _ := testTerminalManager(t)

	// Try starting in /etc (outside allowed root)
	_, err := tm.Start("agent-1", "szakdoga", "test-cwd", "echo hi", "/etc", "ch-1", "root-1", nil)
	if err == nil {
		t.Fatal("expected cwd outside root to be refused, got nil error")
	}
	if !strings.Contains(err.Error(), "outside allowed scope") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestTerminalStartFinishExitCode(t *testing.T) {
	tm, tmp := testTerminalManager(t)

	var finishedMeta *TerminalMeta
	tm.SetOnFinish(func(m *TerminalMeta) {
		finishedMeta = m
	})

	meta, err := tm.Start("agent-1", "@szakdoga", "build-task", "echo 'hello world'; exit 42", tmp, "ch-1", "thread-1", nil)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if meta.Name != "build-task" || meta.Handle != "szakdoga" {
		t.Errorf("unexpected meta: %+v", meta)
	}

	// Poll watcher until finished
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tm.WatchTick()
		m, getErr := tm.Get("szakdoga", "build-task")
		if getErr == nil && m.FinishedAt != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	m, err := tm.Get("szakdoga", "build-task")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if m.FinishedAt == nil {
		t.Fatal("terminal did not finish in time")
	}
	if m.ExitCode == nil || *m.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %v", m.ExitCode)
	}
	if m.Status() != "error" {
		t.Errorf("expected status 'error', got %s", m.Status())
	}
	if finishedMeta == nil || finishedMeta.ExitCode == nil || *finishedMeta.ExitCode != 42 {
		t.Errorf("onFinish callback not invoked or wrong exit code: %+v", finishedMeta)
	}

	// Read output
	out, err := tm.Output("szakdoga", "build-task", 100)
	if err != nil {
		t.Fatalf("Output failed: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %q", out)
	}
}

func TestTerminalLimits(t *testing.T) {
	tm, tmp := testTerminalManager(t)

	// Start 5 terminals for agent-1 (max per agent)
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("task-%d", i)
		_, err := tm.Start("agent-1", "bot1", name, "sleep 30", tmp, "ch-1", "", nil)
		if err != nil {
			t.Fatalf("failed starting terminal %d: %v", i, err)
		}
	}

	// 6th terminal for bot1 should fail limit
	_, err := tm.Start("agent-1", "bot1", "task-6", "sleep 30", tmp, "ch-1", "", nil)
	if err == nil || !strings.Contains(err.Error(), "agent limit reached") {
		t.Fatalf("expected agent limit error, got: %v", err)
	}

	// Starting existing running name should fail
	_, err = tm.Start("agent-1", "bot1", "task-1", "sleep 30", tmp, "ch-1", "", nil)
	if err == nil || !strings.Contains(err.Error(), "is already running") {
		t.Fatalf("expected already running error, got: %v", err)
	}

	// Fill machine to 20 terminals
	for agentIdx := 2; agentIdx <= 4; agentIdx++ {
		handle := fmt.Sprintf("bot%d", agentIdx)
		for tIdx := 1; tIdx <= 5; tIdx++ {
			name := fmt.Sprintf("task-%d", tIdx)
			_, err := tm.Start(fmt.Sprintf("agent-%d", agentIdx), handle, name, "sleep 30", tmp, "ch-1", "", nil)
			if err != nil {
				t.Fatalf("failed starting terminal for %s/%s: %v", handle, name, err)
			}
		}
	}

	// Total is now 20 running. Next terminal on host must fail host limit
	_, err = tm.Start("agent-5", "bot5", "task-1", "sleep 30", tmp, "ch-1", "", nil)
	if err == nil || !strings.Contains(err.Error(), "machine limit reached") {
		t.Fatalf("expected machine limit error, got: %v", err)
	}
}

func TestTerminalHaltKills(t *testing.T) {
	tm, tmp := testTerminalManager(t)

	_, err := tm.Start("agent-1", "bot1", "long-run", "sleep 60", tmp, "ch-1", "", nil)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify running
	m, err := tm.Get("bot1", "long-run")
	if err != nil || m.FinishedAt != nil {
		t.Fatalf("expected running, got: %+v, err: %v", m, err)
	}

	// Halt agent
	tm.KillAllForHandle("bot1", "halt")

	m, err = tm.Get("bot1", "long-run")
	if err != nil {
		t.Fatalf("Get after kill failed: %v", err)
	}
	if m.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
	if m.KilledBy != "halt" {
		t.Errorf("expected killed_by 'halt', got %q", m.KilledBy)
	}
	if m.Status() != "killed" {
		t.Errorf("expected status 'killed', got %s", m.Status())
	}
}

func TestTerminalOutputCappedLinesAndSize(t *testing.T) {
	tm, tmp := testTerminalManager(t)

	// Generate 1500 lines
	script := "for i in $(seq 1 1500); do echo \"line $i\"; done"
	_, err := tm.Start("agent-1", "bot", "many-lines", script, tmp, "ch", "", nil)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tm.WatchTick()
		m, err := tm.Get("bot", "many-lines")
		if err == nil && m.FinishedAt != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Read with max 1000 lines cap
	out, err := tm.Output("bot", "many-lines", 2000)
	if err != nil {
		t.Fatalf("Output failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1000 {
		t.Fatalf("expected capped at 1000 lines, got %d lines", len(lines))
	}
	if !strings.HasPrefix(lines[0], "line 501") {
		t.Errorf("expected first line to be line 501, got %s", lines[0])
	}
	if lines[len(lines)-1] != "line 1500" {
		t.Errorf("expected last line to be line 1500, got %s", lines[len(lines)-1])
	}

	// Verify trimOutputFile trims files > 1 MB
	logPath := filepath.Join(tm.terminalDir("bot", "many-lines"), "big.log")
	bigData := strings.Repeat("A", 1024*1024+500) + "\nlast line\n"
	if err := os.WriteFile(logPath, []byte(bigData), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := trimOutputFile(logPath, 1024*1024); err != nil {
		t.Fatalf("trimOutputFile failed: %v", err)
	}
	fi, err := os.Stat(logPath)
	if err != nil || fi.Size() > 1024*1024 {
		t.Errorf("expected trimmed size <= 1MB, got %d, err %v", fi.Size(), err)
	}
}

func TestTerminalFilterOutput(t *testing.T) {
	raw := "\x1b[31;1mHiba!\x1b[0m \tárvíztűrő fúrótükörgép\x00\x07\r\n"
	filtered := FilterTerminalOutput(raw)
	want := "Hiba! \tárvíztűrő fúrótükörgép\r\n"
	if filtered != want {
		t.Fatalf("FilterTerminalOutput mismatch:\n got: %q\nwant: %q", filtered, want)
	}
}

func TestTerminalAdoptionAfterRestart(t *testing.T) {
	skipIfNoTmux(t)
	tmp := t.TempDir()
	roots := []string{tmp}
	stateDir := filepath.Join(tmp, "state")
	sock := fmt.Sprintf("colonyhall-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", sock, "kill-server").Run()
	})

	// Guard 1 starts a long running terminal
	tm1 := NewTerminalManager(stateDir, roots, tmp)
	tm1.SetSocketName(sock)

	_, err := tm1.Start("agent-1", "bot", "adopt-test", "sleep 60", tmp, "ch", "", nil)
	if err != nil {
		t.Fatalf("tm1 Start failed: %v", err)
	}

	// Guard 2 starts up (simulating restart)
	tm2 := NewTerminalManager(stateDir, roots, tmp)
	tm2.SetSocketName(sock)
	if err := tm2.AdoptExisting(); err != nil {
		t.Fatalf("AdoptExisting failed: %v", err)
	}

	// Verify terminal was adopted as running
	m, err := tm2.Get("bot", "adopt-test")
	if err != nil || m.FinishedAt != nil {
		t.Fatalf("expected adopted terminal to be running, got %+v, err %v", m, err)
	}

	// tm2 should be able to kill it
	if err := tm2.Kill("bot", "adopt-test", "admin"); err != nil {
		t.Fatalf("tm2 Kill failed: %v", err)
	}
	m, err = tm2.Get("bot", "adopt-test")
	if err != nil || m.FinishedAt == nil || m.KilledBy != "admin" {
		t.Fatalf("expected killed by admin, got %+v, err %v", m, err)
	}
}

func TestTerminal7DayCleanupWithFakeClock(t *testing.T) {
	tm, tmp := testTerminalManager(t)

	mockTime := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tm.SetClock(func() time.Time { return mockTime })

	_, err := tm.Start("agent-1", "bot", "aged-task", "echo ok", tmp, "ch", "", nil)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Let it finish
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tm.WatchTick()
		m, err := tm.Get("bot", "aged-task")
		if err == nil && m.FinishedAt != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	termDir := tm.terminalDir("bot", "aged-task")
	if _, err := os.Stat(termDir); err != nil {
		t.Fatalf("expected termDir to exist, got: %v", err)
	}

	// Advance clock by 6 days: should NOT be cleaned up
	mockTime = mockTime.Add(6 * 24 * time.Hour)
	tm.WatchTick()
	if _, err := os.Stat(termDir); err != nil {
		t.Fatalf("termDir removed too early: %v", err)
	}

	// Advance clock past 7 days (7 days + 1 hour): should be cleaned up
	mockTime = mockTime.Add(25 * time.Hour)
	tm.WatchTick()
	if _, err := os.Stat(termDir); !os.IsNotExist(err) {
		t.Fatalf("expected termDir to be removed after 7 days, got err: %v", err)
	}
}

func TestTerminalSurvivesProcessGroupKill(t *testing.T) {
	skipIfNoTmux(t)
	tmp := t.TempDir()
	roots := []string{tmp}
	stateDir := filepath.Join(tmp, "state")
	sock := fmt.Sprintf("colonyhall-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", sock, "kill-server").Run()
	})

	tm := NewTerminalManager(stateDir, roots, tmp)
	tm.SetSocketName(sock)

	// Simulate an agent run process in its own process group
	// The process starts a terminal via tm.Start, then waits
	readyFile := filepath.Join(tmp, "ready")
	runnerScript := fmt.Sprintf(`
		%s
		echo ready > %s
		sleep 100
	`, "", readyFile)

	// Start sub-process with setpgid
	cmd := exec.Command("sh", "-c", runnerScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start failed: %v", err)
	}
	pgid := cmd.Process.Pid

	// Inside this run's scope, the terminal is started
	_, err := tm.Start("agent-1", "bot", "survivor", "sleep 60", tmp, "ch", "", nil)
	if err != nil {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		t.Fatalf("Start failed: %v", err)
	}

	// Verify terminal is running
	session := tm.sessionName("bot", "survivor")
	if !tm.hasSession(session) {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		t.Fatal("expected tmux session to be alive")
	}

	// Now simulate the end of the run: killing the whole process group with SIGKILL
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	_ = cmd.Wait() // runner is dead

	// Check if tmux session survived the death of the process group!
	if !tm.hasSession(session) {
		t.Fatal("tmux session was killed when runner process group died! It should have survived.")
	}

	// Clean up
	_ = tm.Kill("bot", "survivor", "test-cleanup")
}

// Round 40: a terminal name from the server cannot reach another agent's
// folder (or any other output.log).
func TestTerminalRefsCannotTraverse(t *testing.T) {
	state := t.TempDir()
	tm := NewTerminalManager(state, nil, "")
	other := filepath.Join(state, "terminals", "other", "build")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "output.log"), []byte("other agent's secret output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../other/build", "../../terminals/other/build", "x/../../other/build"} {
		if out, err := tm.Output("bot", name, 100); err == nil || out != "" {
			t.Errorf("Output(%q) read %q, %v", name, out, err)
		}
		if _, err := tm.Get("bot", name); err == nil {
			t.Errorf("Get(%q) succeeded", name)
		}
		if err := tm.Kill("bot", name, "admin"); !errors.Is(err, ErrBadTerminalRef) {
			t.Errorf("Kill(%q): %v", name, err)
		}
		if err := tm.Send("bot", name, "ls"); !errors.Is(err, ErrBadTerminalRef) {
			t.Errorf("Send(%q): %v", name, err)
		}
	}
	if _, err := tm.Output("../other", "build", 100); err == nil {
		t.Error("a handle with a path in it was accepted")
	}
	if out, err := tm.Output("other", "build", 100); err != nil || out == "" {
		t.Fatalf("the agent's own terminal still reads: %q %v", out, err)
	}
}
