package guard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gitTimeout is the timeout for each git command run by the guard.
const gitTimeout = 30 * time.Second

func gitExecRaw(ctx context.Context, dir string, args ...string) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmdArgs := append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false"}, args...)
	cmd := exec.CommandContext(tctx, "git", cmdArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0")
	return cmd.CombinedOutput()
}

// gitExec runs a git command with hooks and user/system git config hooks disabled:
// -c core.hooksPath=/dev/null, GIT_CONFIG_NOSYSTEM=1, GIT_CONFIG_GLOBAL=/dev/null, no shell, arguments as slice, timeout.
// core.fsmonitor is off too: a repo's own config could name a command there,
// and git would run it on checkout or status.
func gitExec(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitExecRaw(ctx, dir, args...)
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return trimmed, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmed)
		}
		return trimmed, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return trimmed, nil
}

// emptyTree is git's well-known empty tree object.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// WorktreeGitDir returns the explicit path to the worktree's git administrative
// directory inside the repository (<repo>/.git/worktrees/<handle>).
func WorktreeGitDir(repoPath, handle string) string {
	cleanRepo := filepath.Clean(repoPath)
	gitDir := filepath.Join(cleanRepo, ".git")
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		return filepath.Join(gitDir, "worktrees", handle)
	}
	return filepath.Join(cleanRepo, "worktrees", handle)
}

// SandboxedGitExec executes a git command targeting a sandboxed worktree, passing
// --git-dir and --work-tree explicitly so git never reads the worktree's .git pointer file.
func SandboxedGitExec(ctx context.Context, repoPath, worktreePath, handle string, args ...string) (string, error) {
	gitDir := WorktreeGitDir(repoPath, handle)
	// Attributes come from the empty tree, so a .gitattributes written by the
	// agent cannot switch on a filter, diff or textconv driver: a second line
	// of defence behind the read-only .git in the sandbox.
	fullArgs := append([]string{
		"-c", "core.quotepath=false",
		"-c", "core.attributesFile=/dev/null",
		"--attr-source=" + emptyTree,
		"--git-dir=" + gitDir,
		"--work-tree=" + worktreePath,
	}, args...)
	return gitExec(ctx, worktreePath, fullArgs...)
}

// IsGitRepo reports whether dir points to a valid git repository.
func IsGitRepo(ctx context.Context, dir string) bool {
	_, err := gitExec(ctx, dir, "rev-parse", "--git-dir")
	return err == nil
}

// EnsureWorktree ensures the agent has a git worktree at <repo>-worktrees/<handle>
// on branch colony/<handle>/work.
// It creates the branch from the repo's current default branch if missing,
// reuses an existing worktree if present, and never touches the main working tree.
func EnsureWorktree(ctx context.Context, repoPath, handle string) (string, string, error) {
	cleanRepo := filepath.Clean(repoPath)
	if !IsGitRepo(ctx, cleanRepo) {
		return "", "", fmt.Errorf("not a git repository")
	}

	worktreeDir := filepath.Join(cleanRepo+"-worktrees", handle)
	branch := "colony/" + handle + "/work"

	// Check if an existing worktree directory is present on disk.
	if fi, err := os.Stat(worktreeDir); err == nil && fi.IsDir() {
		if _, err := gitExec(ctx, worktreeDir, "rev-parse", "--is-inside-work-tree"); err == nil {
			// Worktree directory already exists and is inside a worktree.
			head, err := gitExec(ctx, worktreeDir, "symbolic-ref", "--short", "HEAD")
			if err == nil && head == branch {
				return worktreeDir, branch, nil
			}
			// If on a different branch, switch to the target branch.
			if branchExists(ctx, cleanRepo, branch) {
				if _, err := gitExec(ctx, worktreeDir, "checkout", branch); err == nil {
					return worktreeDir, branch, nil
				}
			} else {
				def := defaultBranch(ctx, cleanRepo)
				if _, err := gitExec(ctx, worktreeDir, "checkout", "-b", branch, def); err == nil {
					return worktreeDir, branch, nil
				}
			}
		}
	}

	// Prune missing or stale worktree metadata in the repository.
	_, _ = gitExec(ctx, cleanRepo, "worktree", "prune")

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(worktreeDir), 0o755); err != nil {
		return "", "", fmt.Errorf("mkdir worktrees parent: %w", err)
	}

	if branchExists(ctx, cleanRepo, branch) {
		if _, err := gitExec(ctx, cleanRepo, "worktree", "add", worktreeDir, branch); err != nil {
			return "", "", fmt.Errorf("git worktree add: %w", err)
		}
	} else {
		def := defaultBranch(ctx, cleanRepo)
		if _, err := gitExec(ctx, cleanRepo, "worktree", "add", "-b", branch, worktreeDir, def); err != nil {
			return "", "", fmt.Errorf("git worktree add -b: %w", err)
		}
	}

	return worktreeDir, branch, nil
}

// branchExists reports whether a local branch exists in the repo.
func branchExists(ctx context.Context, repo, branch string) bool {
	_, err := gitExec(ctx, repo, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil
}

// defaultBranch determines the default branch of the repository.
// It checks origin/HEAD, then the current HEAD branch of the repo,
// then refs/heads/main or refs/heads/master, falling back to HEAD.
func defaultBranch(ctx context.Context, repo string) string {
	if out, err := gitExec(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return strings.TrimPrefix(out, "origin/")
	}
	if out, err := gitExec(ctx, repo, "symbolic-ref", "--short", "HEAD"); err == nil && out != "" {
		return out
	}
	if _, err := gitExec(ctx, repo, "rev-parse", "--verify", "refs/heads/main"); err == nil {
		return "main"
	}
	if _, err := gitExec(ctx, repo, "rev-parse", "--verify", "refs/heads/master"); err == nil {
		return "master"
	}
	return "HEAD"
}

// checkAndCommitWorktree checks changes in a sandboxed agent's worktree after a run.
// If any change violates safety rules (secret file, >10MB, external symlink), it refuses to commit
// and returns a one-line explanation for the chat reply.
// If valid changes exist, it commits them with the guard's author and message and returns
// a line formatted as: "🌿 commit <sha> on colony/<handle>/work (<n> files)".
// If no files were changed, it returns an empty string.
func (a *Agent) checkAndCommitWorktree(ctx context.Context, it PromptItem, task, worktreeDir string) string {
	if !a.Sandbox || a.WorktreeRepo == "" || worktreeDir == "" {
		return ""
	}
	handle := a.Cfg.Name
	if handle == "" {
		handle = a.getMe().Handle
	}

	// 1. Check status with porcelain -u
	statusOut, err := SandboxedGitExec(ctx, a.WorktreeRepo, worktreeDir, handle, "status", "--porcelain", "-u")
	if err != nil {
		a.Log.Warn("guard: could not check git status for sandboxed worktree", "agent", a.logName(), "err", err)
		return "⚠️ Commit failed: could not check git status; changes left uncommitted in the worktree."
	}
	statusOut = strings.TrimSpace(statusOut)
	if statusOut == "" {
		return ""
	}

	lines := strings.Split(statusOut, "\n")
	var changedFiles []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if len(l) < 3 {
			continue
		}
		filePath := strings.TrimSpace(l[2:])
		if idx := strings.Index(filePath, " -> "); idx != -1 {
			filePath = filePath[idx+4:]
		}
		filePath = strings.Trim(filePath, "\"")
		if filePath != "" {
			changedFiles = append(changedFiles, filePath)
		}
	}
	if len(changedFiles) == 0 {
		return ""
	}

	cleanWorktree := filepath.Clean(worktreeDir)

	// 2. Safety checks on changed files
	for _, relPath := range changedFiles {
		// Secret name check
		base := filepath.Base(relPath)
		if isSecretName(base) {
			a.Log.Warn("guard: commit refused: changed file matches secret pattern", "agent", a.logName(), "file", relPath)
			return "⚠️ Commit refused: changed file matches secret pattern; changes left uncommitted in the worktree."
		}
		for _, part := range strings.Split(filepath.ToSlash(relPath), "/") {
			if isSecretName(part) {
				a.Log.Warn("guard: commit refused: path contains secret pattern", "agent", a.logName(), "file", relPath)
				return "⚠️ Commit refused: changed file matches secret pattern; changes left uncommitted in the worktree."
			}
		}

		fullPath := filepath.Join(cleanWorktree, relPath)
		fi, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Deleted file, allowed unless it was a symlink pointing outside
				continue
			}
			a.Log.Warn("guard: commit refused: could not stat changed file", "agent", a.logName(), "file", relPath, "err", err)
			return "⚠️ Commit refused: could not stat changed file; changes left uncommitted in the worktree."
		}

		// Symlink pointing outside the worktree check
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(fullPath)
			if err != nil {
				a.Log.Warn("guard: commit refused: invalid symlink", "agent", a.logName(), "file", relPath, "err", err)
				return "⚠️ Commit refused: invalid symlink; changes left uncommitted in the worktree."
			}
			cleanTarget := filepath.Clean(target)
			if !within(cleanTarget, cleanWorktree) && cleanTarget != cleanWorktree {
				a.Log.Warn("guard: commit refused: symlink points outside worktree", "agent", a.logName(), "file", relPath, "target", target)
				return "⚠️ Commit refused: symlink points outside worktree; changes left uncommitted in the worktree."
			}
		}

		// Check if any parent component in the path is an external symlink
		if realPath, err := filepath.EvalSymlinks(fullPath); err == nil {
			cleanReal := filepath.Clean(realPath)
			if !within(cleanReal, cleanWorktree) && cleanReal != cleanWorktree {
				a.Log.Warn("guard: commit refused: path resolves outside worktree", "agent", a.logName(), "file", relPath, "real", cleanReal)
				return "⚠️ Commit refused: symlink points outside worktree; changes left uncommitted in the worktree."
			}
		}

		// Max size 10 MB (10 * 1024 * 1024 bytes) check
		if fi.Mode().IsRegular() && fi.Size() > 10*1024*1024 {
			a.Log.Warn("guard: commit refused: file exceeds 10 MB limit", "agent", a.logName(), "file", relPath, "size", fi.Size())
			return "⚠️ Commit refused: file exceeds 10 MB limit; changes left uncommitted in the worktree."
		}
	}

	// 3. git add -A
	if _, err := SandboxedGitExec(ctx, a.WorktreeRepo, worktreeDir, handle, "add", "-A"); err != nil {
		a.Log.Warn("guard: git add failed", "agent", a.logName(), "err", err)
		return "⚠️ Commit failed: git add failed; changes left uncommitted in the worktree."
	}

	// 4. git commit
	taskSubject := clipSubject(task, 60)
	firstLine := fmt.Sprintf("colony(%s): %s", handle, taskSubject)
	where := "#" + it.Channel
	if it.DM {
		where = "DM"
	}
	bodyLine := fmt.Sprintf("Requested in %s by @%s", where, it.Author)
	commitMsg := firstLine + "\n\n" + bodyLine
	author := fmt.Sprintf("%s (Colonyhall sandbox) <%s@colonyhall.local>", handle, handle)

	// The identity is set here: the user's global config is not read, and a
	// repository often has no user.name of its own.
	if _, err := SandboxedGitExec(ctx, a.WorktreeRepo, worktreeDir, handle,
		"-c", "user.name="+handle+" (Colonyhall sandbox)", "-c", "user.email="+handle+"@colonyhall.local", "-c", "commit.gpgsign=false",
		"commit", "-m", commitMsg, "--author="+author); err != nil {
		a.Log.Warn("guard: git commit failed", "agent", a.logName(), "err", err)
		return "⚠️ Commit failed: git commit failed; changes left uncommitted in the worktree."
	}

	// 5. Short commit sha and files count
	sha, err := SandboxedGitExec(ctx, a.WorktreeRepo, worktreeDir, handle, "rev-parse", "--short", "HEAD")
	if err != nil {
		a.Log.Warn("guard: could not read commit sha", "agent", a.logName(), "err", err)
		sha = "HEAD"
	}
	branch := a.WorktreeBranch
	if branch == "" {
		branch = "colony/" + handle + "/work"
	}
	return fmt.Sprintf("🌿 commit %s on %s (%d files)", sha, branch, len(changedFiles))
}

// clipSubject shortens s to at most maxRunes, flattening newlines and whitespace.
func clipSubject(s string, maxRunes int) string {
	fields := strings.Fields(s)
	flat := strings.Join(fields, " ")
	runes := []rune(flat)
	if len(runes) <= maxRunes {
		return flat
	}
	return string(runes[:maxRunes])
}
