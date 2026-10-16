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

// gitExec runs a git command with hooks and user/system git config hooks disabled:
// -c core.hooksPath=/dev/null, GIT_CONFIG_NOSYSTEM=1, no shell, arguments as slice, timeout.
func gitExec(ctx context.Context, dir string, args ...string) (string, error) {
	tctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmdArgs := append([]string{"-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(tctx, "git", cmdArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return trimmed, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, trimmed)
		}
		return trimmed, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return trimmed, nil
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
