package guard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A sandboxed agent can write its repository, so a .git pointer it planted
// must not be followed: neither by git (a crafted config) nor by the
// exclude write (a file outside the repository).
func TestCodeGraphRefusesPlantedGitPointer(t *testing.T) {
	cg, _ := newTestCodeGraph(t)
	outside := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", outside).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: "+filepath.Join(outside, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noWrap := func(string, string, []string) (string, []string, []string, error) {
		t.Fatal("git ran for a refused repository")
		return "", nil, nil, nil
	}
	if _, _, err := cg.prepare(context.Background(), repo, noWrap); err == nil {
		t.Fatal("a planted .git pointer was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, ".git", "info", "exclude")); err == nil {
		raw, _ := os.ReadFile(filepath.Join(outside, ".git", "info", "exclude"))
		if strings.Contains(string(raw), codeGraphExclude) {
			t.Fatal("the guard wrote into the git folder the pointer named")
		}
	}
}

// A linked info folder (or exclude file) must not redirect the guard's write.
func TestCodeGraphExcludeDoesNotFollowLinks(t *testing.T) {
	target := t.TempDir()
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	info := filepath.Join(repo, ".git", "info")
	if err := os.RemoveAll(info); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, info); err != nil {
		t.Fatal(err)
	}
	gitDir, err := trustedGitDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := excludeCodeGraph(context.Background(), repo, gitDir); err == nil {
		t.Fatal("a linked info folder was accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "exclude")); err == nil {
		t.Fatal("the exclude file was written through the link")
	}

	// The exclude file itself as a link.
	if err := os.Remove(info); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(info, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(target, "victim")
	if err := os.WriteFile(victim, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(info, "exclude")); err != nil {
		t.Fatal(err)
	}
	if err := excludeCodeGraph(context.Background(), repo, gitDir); err == nil {
		t.Fatal("a linked exclude file was accepted")
	}
	if raw, _ := os.ReadFile(victim); string(raw) != "keep\n" {
		t.Fatalf("the link target was changed: %q", raw)
	}
}

// A real worktree (pointer with git's back link) is still indexed, and the
// exclude goes into the repository's own .git folder.
func TestCodeGraphAcceptsRealWorktree(t *testing.T) {
	repo := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(err, string(out))
		}
	}
	run(repo, "init", "-q")
	run(repo, "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	run(repo, "worktree", "add", "-q", "-b", "work", wt)
	gitDir, err := trustedGitDir(wt)
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if got, _ := filepath.EvalSymlinks(gitDir); got != real {
		t.Fatalf("git folder %q, want %q", gitDir, real)
	}
	if err := excludeCodeGraph(context.Background(), wt, gitDir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(raw), codeGraphExclude) {
		t.Fatalf("exclude %q, %v", raw, err)
	}
}

// In a sandboxed agent's repository git never runs on the host: the agent
// may have created .git itself, with commands in its config. The query goes
// through the indexer's sandbox wrap.
func TestCodeGraphGitRunsInSandboxForSandboxedRepo(t *testing.T) {
	cg, _ := newTestCodeGraph(t)
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	var calls [][]string
	wrap := func(r, name string, args []string) (string, []string, []string, error) {
		calls = append(calls, append([]string{name}, args...))
		return "/bin/sh", []string{"-c", `printf 'a\0b\0c\0'`}, nil, nil
	}
	files, args, err := cg.prepare(context.Background(), repo, wrap)
	if err != nil {
		t.Fatal(err)
	}
	if files != 3 || len(args) == 0 || args[0] != "init" {
		t.Fatalf("files %d, args %q", files, args)
	}
	if len(calls) != 1 || filepath.Base(calls[0][0]) != "git" ||
		!slices.Contains(calls[0], "core.fsmonitor=false") || !slices.Contains(calls[0], "ls-files") {
		t.Fatalf("git did not go through the sandbox wrap: %q", calls)
	}
}

// The same with a real bwrap: git works inside the indexer's sandbox.
func TestCodeGraphSandboxedGitWithBwrap(t *testing.T) {
	sbx := DetectSandbox(context.Background(), t.TempDir(), t.TempDir())
	if sbx == nil {
		t.Skip("no working bwrap")
	}
	sbx.Home = t.TempDir()
	cg, _ := newTestCodeGraph(t)
	repo := initTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrap := func(r, name string, args []string) (string, []string, []string, error) {
		return sbx.IndexCommand(cg, r, name, args)
	}
	files, _, err := cg.prepare(context.Background(), repo, wrap)
	if err != nil {
		t.Fatal(err)
	}
	if files < 1 {
		t.Fatalf("git ls-files in the sandbox found %d files", files)
	}
}
