# Handoff prompt: direct mode review fixes (for Antigravity CLI)

```sh
cd /mnt/g/Projects/new-chat   # stay on branch feature/direct-mode
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/02-direct-mode-fixes-prompt.md)"
```

---

You implemented direct mode on branch `feature/direct-mode` (see `docs/handoff/01-direct-mode-report.md`). The reviewer (Claude Code) found the issues below. Same ground rules as in `docs/handoff/01-direct-mode-prompt.md` section 2 (read it again): stay on this branch, no real `claude`/`agy` calls, contract-first, tests for memory and Postgres, `make test`, `make lint`, `make test-pg`, `make gen-check` green, one commit per fix with a test that would have failed before the fix.

## Fix 1 (security, most important): the server must not be able to switch the sandbox off

Today `sandbox: false` / direct mode is decided only by the server. A compromised server or admin account could turn any agent on a machine into an unsandboxed, full-access agent. The design principle (docs/01-terv.md, "Gépek, mappahozzáférés és sandbox") is that the machine owner sets the upper bound in `guard.yaml` and the server cannot extend it.

- Add `allow_unsandboxed` (bool, default **false**) to the host-mode `guard.yaml` (`internal/guard/config.go`, `guard.example.yaml` with a comment explaining it).
- In `Supervisor.reconcile`/`startAgent`: an agent with `sandbox=false` (which includes direct mode as configured today) is **not started** unless `allow_unsandboxed: true`; add it to `Rejected` with a clear reason ("unsandboxed agents are not allowed on this machine (guard.yaml allow_unsandboxed)") and log it.
- Report the setting in the host report (add an `allow_unsandboxed` boolean to `HostReport` in `api/openapi.yaml`, the guard client and `hosts.Report`) so the dev page can show it; show it in the "Gépek" panel.
- Tests: unit/e2e proving a direct, unsandboxed assignment is refused and reported when the flag is off, and runs when it is on.

## Fix 2 (bug): a failed or killed first run must not break the thread's session

`runDirectGroup` records `state.ClaudeSessions[sessionID] = true` only after a successful run. Claude Code creates the session as soon as the run starts. If the first run fails or is killed (timeout, emergency stop), the retry passes `--session-id <same uuid>` again, which Claude rejects because the id already exists; after 3 attempts the items are dropped and the thread can never continue.

- Decide `Resume` as: `state.ClaudeSessions[id] || claudeSessionExists(id)`, where `claudeSessionExists` checks for a file `<id>.jsonl` under `$CLAUDE_CONFIG_DIR/projects/*/` or `~/.claude/projects/*/` (use `filepath.Glob`; the id is a UUID you generated, so it is safe to put in the pattern).
- Also record the session in the state file when the run produced output with a `session_id`, even if `is_error` was true.
- Tests: a first run that fails after "creating" the session (simulate by creating the `.jsonl` file in a temp `CLAUDE_CONFIG_DIR`) → the next run uses `--resume`.

## Fix 3 (consistency): compact + unsandboxed + empty root_dir

The server accepts `mode=compact, sandbox=false, root_dir=""`, the guard then refuses it (`CheckGrant("")`). Make the server require a `root_dir` whenever `sandbox` is false (validation error, e2e contract test), so the two sides agree.

## Fix 4 (duplicate replies)

In direct mode the guard posts the run's final text into the thread. If the agent also replies there via the Colonyhall MCP `send` tool, the thread gets two answers. Add one sentence to `DirectPrompt`: your final answer is posted to this thread for you; do not post it yourself with the colonyhall tools (use them only to message other agents or other channels). Update `TestDirectPrompt`.

## Fix 5 (cleanup)

If the guard crashes during a run, the temporary `mcp-*.json` (with an agent token, valid up to 1 h) stays in the agent's folder. On agent start (`startAgent`), remove stale `mcp-*.json` files in that folder. Test it.

## Report

Append a section "2. kör: review javítások" to `docs/handoff/01-direct-mode-report.md` (Hungarian): commits, decisions, final command results. Then stop and print a 5-line summary. Do not merge.
