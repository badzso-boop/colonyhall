package guard

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(out))
		}
	}

	git("init", "-b", "main")
	git("config", "user.name", "Test Runner")
	git("config", "user.email", "test@example.com")

	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# My Repo\nInitial content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "-m", "initial commit")

	return repoDir
}

func TestEnsureWorktree_CreationAndReuse(t *testing.T) {
	ctx := context.Background()
	repoDir := initTestRepo(t)

	// Record main repo state
	mainHeadBefore, err := gitExec(ctx, repoDir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	mainBranchBefore, err := gitExec(ctx, repoDir, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	mainStatusBefore, err := gitExec(ctx, repoDir, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}

	// 1. First creation
	wtPath, branch, err := EnsureWorktree(ctx, repoDir, "agent-claude")
	if err != nil {
		t.Fatalf("EnsureWorktree failed: %v", err)
	}

	expectedPath := filepath.Join(repoDir+"-worktrees", "agent-claude")
	if wtPath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, wtPath)
	}
	expectedBranch := "colony/agent-claude/work"
	if branch != expectedBranch {
		t.Errorf("expected branch %s, got %s", expectedBranch, branch)
	}

	// Verify worktree exists and is on the branch
	wtBranch, err := gitExec(ctx, wtPath, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		t.Fatalf("failed to get worktree branch: %v", err)
	}
	if wtBranch != expectedBranch {
		t.Errorf("worktree is on branch %s, want %s", wtBranch, expectedBranch)
	}

	// Verify main tree was completely untouched
	mainHeadAfter, err := gitExec(ctx, repoDir, "rev-parse", "HEAD")
	if err != nil || mainHeadAfter != mainHeadBefore {
		t.Errorf("main HEAD changed: before=%s after=%s", mainHeadBefore, mainHeadAfter)
	}
	mainBranchAfter, err := gitExec(ctx, repoDir, "symbolic-ref", "--short", "HEAD")
	if err != nil || mainBranchAfter != mainBranchBefore {
		t.Errorf("main branch changed: before=%s after=%s", mainBranchBefore, mainBranchAfter)
	}
	mainStatusAfter, err := gitExec(ctx, repoDir, "status", "--porcelain")
	if err != nil || mainStatusAfter != mainStatusBefore {
		t.Errorf("main status changed: before=%s after=%s", mainStatusBefore, mainStatusAfter)
	}

	// 2. Reuse existing worktree
	wtPath2, branch2, err := EnsureWorktree(ctx, repoDir, "agent-claude")
	if err != nil {
		t.Fatalf("EnsureWorktree reuse failed: %v", err)
	}
	if wtPath2 != wtPath || branch2 != branch {
		t.Errorf("reused worktree mismatch: got (%s, %s), want (%s, %s)", wtPath2, branch2, wtPath, branch)
	}

	// 3. Make a commit in worktree and ensure main repo remains untouched
	fileInWt := filepath.Join(wtPath, "feature.txt")
	if err := os.WriteFile(fileInWt, []byte("feature work"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitExec(ctx, wtPath, "add", "feature.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitExec(ctx, wtPath, "commit", "-m", "work in progress"); err != nil {
		t.Fatal(err)
	}

	mainStatusAfterCommit, err := gitExec(ctx, repoDir, "status", "--porcelain")
	if err != nil || mainStatusAfterCommit != "" {
		t.Errorf("main repo dirty after worktree commit: %s", mainStatusAfterCommit)
	}
	mainHeadAfterCommit, err := gitExec(ctx, repoDir, "rev-parse", "HEAD")
	if err != nil || mainHeadAfterCommit != mainHeadBefore {
		t.Errorf("main HEAD changed after worktree commit: before=%s after=%s", mainHeadBefore, mainHeadAfterCommit)
	}
}

func TestEnsureWorktree_MaliciousHookIgnored(t *testing.T) {
	ctx := context.Background()
	repoDir := initTestRepo(t)

	// Create a malicious hook in .git/hooks/post-checkout
	hooksDir := filepath.Join(repoDir, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	markerFile := filepath.Join(t.TempDir(), "malicious_hook_executed.marker")
	hookScript := "#!/bin/sh\ntouch " + markerFile + "\n"
	postCheckout := filepath.Join(hooksDir, "post-checkout")
	if err := os.WriteFile(postCheckout, []byte(hookScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// Configure repo to use core.hooksPath
	cmd := exec.Command("git", "config", "core.hooksPath", hooksDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config failed: %v: %s", err, string(out))
	}

	// Run EnsureWorktree
	wtPath, branch, err := EnsureWorktree(ctx, repoDir, "agent-malicious")
	if err != nil {
		t.Fatalf("EnsureWorktree failed: %v", err)
	}
	if branch != "colony/agent-malicious/work" {
		t.Fatalf("unexpected branch: %s", branch)
	}
	_ = wtPath

	// Verify the hook never ran
	if _, err := os.Stat(markerFile); err == nil {
		t.Fatalf("MALICIOUS HOOK WAS EXECUTED! Guard failed to bypass hooks")
	}
}

func TestEnsureWorktree_NotAGitRepo(t *testing.T) {
	ctx := context.Background()
	emptyDir := t.TempDir()

	_, _, err := EnsureWorktree(ctx, emptyDir, "agent-foo")
	if err == nil {
		t.Fatalf("expected error for non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("expected 'not a git repository' error, got %v", err)
	}
}

func TestSupervisor_WorktreeOutsideRootsRefused(t *testing.T) {
	ctx := context.Background()
	rootsDir := t.TempDir()
	outsideDir := t.TempDir()

	// Init git repo outside machine roots
	outsideRepo := filepath.Join(outsideDir, "foreign-repo")
	if err := os.MkdirAll(outsideRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = outsideRepo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v: %s", err, string(out))
	}

	s := &Supervisor{
		Cfg:   Config{Roots: []string{rootsDir}, SandboxDir: filepath.Join(rootsDir, "sb"), AllowUnsandboxed: true},
		roots: []string{rootsDir},
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	cfg := HostConfig{
		Version: "v1",
		Agents: []HostAgent{
			{
				AgentID:      "11111111-1111-1111-1111-111111111111",
				Handle:       "worker-agent",
				Client:       "claude-code",
				Mode:         "direct",
				WorktreeRepo: outsideRepo,
				Sandbox:      false,
			},
		},
	}

	s.reconcile(ctx, cfg)

	// Agent must NOT be started
	if len(s.agents) != 0 {
		t.Fatalf("expected 0 running agents, got %d", len(s.agents))
	}
	// Rejection must be reported
	if len(s.rejected) != 1 {
		t.Fatalf("expected 1 rejected path, got %d", len(s.rejected))
	}
	if s.rejected[0].AgentID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("rejected agent mismatch: %+v", s.rejected[0])
	}
	if !strings.Contains(s.rejected[0].Reason, "outside the root folders") {
		t.Errorf("expected outside roots reason, got %q", s.rejected[0].Reason)
	}
}

func TestSupervisor_WorktreeReconcileAndReport(t *testing.T) {
	ctx := context.Background()
	rootsDir := t.TempDir()
	repoDir := filepath.Join(rootsDir, "local-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = repoDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Tester")
	cmd.Dir = repoDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = repoDir
	_ = cmd.Run()
	_ = os.WriteFile(filepath.Join(repoDir, "init.txt"), []byte("init"), 0o644)
	cmd = exec.Command("git", "add", "init.txt")
	cmd.Dir = repoDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = repoDir
	_ = cmd.Run()

	agentID := "22222222-2222-2222-2222-222222222222"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "tok", "expires_at": time.Now().Add(time.Hour)})
		case strings.HasSuffix(r.URL.Path, "/me"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": agentID, "handle": "coder", "state": "active"})
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer ts.Close()

	s := &Supervisor{
		Cfg:        Config{Server: ts.URL, Roots: []string{rootsDir}, SandboxDir: filepath.Join(rootsDir, "sb"), AllowUnsandboxed: true},
		host:       &Client{BaseURL: ts.URL, Token: "host-tok", HTTP: ts.Client()},
		roots:      []string{rootsDir},
		agents:     map[string]*managed{},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewAdapter: func(ac AgentConfig) Adapter { return &ClaudeCode{} },
	}

	cfg := HostConfig{
		Version: "v1",
		Agents: []HostAgent{
			{
				AgentID:      "22222222-2222-2222-2222-222222222222",
				Handle:       "coder",
				Client:       "claude-code",
				Mode:         "direct",
				WorktreeRepo: repoDir,
				Sandbox:      false,
			},
		},
	}

	s.reconcile(ctx, cfg)

	if len(s.rejected) != 0 {
		t.Fatalf("unexpected rejections: %+v", s.rejected)
	}
	if len(s.worktrees) != 1 {
		t.Fatalf("expected 1 worktree in supervisor, got %d", len(s.worktrees))
	}
	wt := s.worktrees[0]
	if wt.AgentID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("agent id mismatch: %s", wt.AgentID)
	}
	if wt.Branch != "colony/coder/work" {
		t.Errorf("branch mismatch: %s", wt.Branch)
	}
	expectedWTPath := filepath.Join(repoDir+"-worktrees", "coder")
	if wt.Path != expectedWTPath {
		t.Errorf("path mismatch: got %s, want %s", wt.Path, expectedWTPath)
	}

	// Verify agent configuration
	m := s.agents["22222222-2222-2222-2222-222222222222"]
	if m == nil {
		t.Fatal("agent not started")
	}
	if m.agent.RootDir != expectedWTPath {
		t.Errorf("agent root dir should be worktree path: got %s, want %s", m.agent.RootDir, expectedWTPath)
	}
	if m.agent.WorktreeBranch != "colony/coder/work" {
		t.Errorf("agent WorktreeBranch mismatch: got %s", m.agent.WorktreeBranch)
	}

	// Stop supervisor
	s.stopAll()
}

func TestDirectPrompt_WithWorktreeBranch(t *testing.T) {
	items := []Item{
		{ID: "i1", Channel: "general", Author: "norbi", Body: "Please fix bug"},
	}

	// Without branch
	p1 := DirectPrompt(items)
	if strings.Contains(p1, "worktree") {
		t.Fatalf("plain prompt should not contain worktree text: %s", p1)
	}

	// With branch
	p2 := DirectPromptWithBranch(items, "colony/backend-claude/work")
	expectedText := "You work in your own worktree on branch colony/backend-claude/work; commit there; never commit to the main branch; deliver through a PR when asked."
	if !strings.Contains(p2, expectedText) {
		t.Fatalf("prompt missing expected worktree instruction:\nwant: %s\ngot:\n%s", expectedText, p2)
	}
}
