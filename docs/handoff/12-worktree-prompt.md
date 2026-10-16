# Handoff prompt 12: a git worktree per full-access agent

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/12-worktree-prompt.md)"
```

---

Branch: `feature/worktree`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 6, and "Munkaterület: git worktree agentenként". Read `internal/guard/host.go` (`reconcile`, `startAgent`, root dir checks), `internal/guard/direct.go`, `internal/guard/paths.go`.

## Task

- Assignment field `worktree_repo` (path of a git repository on the host, inside the machine's roots; empty = off), only for direct mode. Contract-first, migration, hosts API, dev page.
- Guard, when it starts an agent with `worktree_repo`:
  - validates the path like a root dir (`CheckGrant`), and that it is a git repository (`git -C <repo> rev-parse --git-dir`);
  - ensures the worktree `<repo>-worktrees/<handle>` on branch `colony/<handle>/work` (create the branch from the repo's current default branch if missing; reuse an existing worktree; never touch the main working tree);
  - runs every git command of its own with hooks and the user's git config hooks disabled: `git -c core.hooksPath=/dev/null …`, `GIT_CONFIG_NOSYSTEM=1`, no shell, arguments as a slice, a timeout;
  - uses the worktree as the run's directory (Claude: cwd; Antigravity: `--add-dir`) instead of `root_dir`;
  - reports the worktree path and branch in the host report (shown on the "Gépek" panel).
- The direct prompt says: you work in your own worktree on branch `colony/<handle>/work`; commit there; never commit to the main branch; deliver through a PR when asked.
- Failure to create the worktree: the agent is not started, the reason is reported (like a refused root dir).
- Tests with real `git` in temp directories (it is installed): worktree created and reused, branch name, main tree untouched, a malicious `core.hooksPath`/hook in the repo is not executed by the guard, repo outside roots refused, not-a-repo refused.

## Docs

`docs/02-muszaki-specifikacio.md` (Őr: worktree), plan section marked, `CLAUDE.md` schedule. Hungarian.
