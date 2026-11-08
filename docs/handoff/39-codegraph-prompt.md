# Handoff prompt 39: CodeGraph for every agent, wired in by the guard

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/codegraph ../new-chat-worktrees/codegraph main
cd ../new-chat-worktrees/codegraph
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/39-codegraph-prompt.md)"
```

---

Branch: `feature/codegraph`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/codegraph`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. Mostly `internal/guard`; next free migration (if you need one, you should not): check `migrations/`.

After each part: `gofmt`, `make test`, `make lint`; `make test-web` for PWA parts. At the end `go test -race ./internal/guard/ ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it.

## Why (Norbi, 2026-11-05)

Agents spend much of their context and many tool calls finding code: `grep`, `glob`, reading file after file. [CodeGraph](https://github.com/colbymchenry/codegraph) builds a local graph of a project's symbols, calls and dependencies (tree-sitter, SQLite with FTS5, `.codegraph/codegraph.db` in the project) and answers "how does X work / where is Y / what does changing Z affect" in one MCP call (`codegraph_explore`), with verbatim source and line numbers.

Measured on the Colonyhall repo (homelab, 2026-11-05): 807 files indexed in 11 s, a 52 MB index; one `explore` question answered in 0.7 s with the right functions, about 25 KB (6–7k tokens).

## State and decisions (binding)

- **Installed** on all three guard machines (PC, homelab, nato), v1.6.2, by Claude Code with the reviewed `install.sh`: the bundle is in `~/.codegraph`, the launcher is `~/.local/bin/codegraph` (not on the non-interactive SSH `PATH` on the homelab: always use the absolute path). **Telemetry is off** on all three (`codegraph telemetry off`).
- **Never run `codegraph install`** (it writes into the global `CLAUDE.md`, adds MCP servers and auto-allow permissions for every agent of the user). **The guard wires CodeGraph in itself**, per run, like the web-read and planner MCP servers.
- **The guard indexes.** Agents never need to run `codegraph init`.
- **Every agent gets it**, the colleague's too (round 38), Claude Code and Antigravity. It is a tool the agent may use when it helps, not a rule.
- **Sandboxed agents:** CodeGraph runs **inside** the sandbox and sees only the granted folders; nothing outside.
- No trial phase: on for everyone once deployed.

## 1. Guard configuration and detection

- `guard.yaml`: `codegraph:` with `bin` (default `~/.local/bin/codegraph` when it exists) and `enabled` (default true when the binary runs). The guard checks `codegraph --version` at start and reports it in the host report (`HostReport.codegraph_version`, contract first; the Machines page shows it).
- Every child process the guard starts for CodeGraph gets `CODEGRAPH_TELEMETRY=0` and `DO_NOT_TRACK=1`, whatever the machine's own setting.
- No binary or `enabled: false`: nothing changes, no error.

## 2. Indexing (the guard)

- **What is indexed:** each agent's git repositories, never a whole folder of projects.
  - The agent's `root_dir` when it is a git repo; its worktrees (round 12); repos under the root that the agent actually works in (detected from its run steps' paths, round 22) are indexed after that run.
  - A root that is a folder of many repos (e.g. `/mnt/g/Projects`) is never indexed as a whole.
- **How:** `codegraph init --local-only <repo>` (keeps `.codegraph/` out of git), then incremental `codegraph sync` before a run in that repo when the index exists. One indexing job at a time per guard, at low CPU and IO priority (`nice`, `ionice`); never blocks or delays a run (a run without an index simply has the tool answer "not indexed").
- **Limits:** `codegraph.max_files` (default e.g. 20 000) skips huge repos with a guard log line; a failed index is retried later, not in a loop.
- **Never committed:** besides `--local-only`, add `.codegraph/` to the sandboxed agents' automatic commit filter (round 18) so a 50 MB database can never be committed, even in a repo whose ignore rules are missing.
- **Machines page:** per machine, the indexed repos (path, files, last indexed) in the host report (contract first; capped list).
- **Tests:** a fake `codegraph` binary records the calls: the right repos, `--local-only`, the env, one job at a time, the `max_files` skip, the commit filter.

## 3. The MCP server in every run

- **Claude Code** (direct, work; not decide, which has no tools): add the server to the run's MCP config:
  `{"command": "<bin>", "args": ["serve", "--mcp"], "env": {"CODEGRAPH_TELEMETRY": "0", "DO_NOT_TRACK": "1"}}`, with `alwaysLoad` so `codegraph_explore` is available from the first prompt (see the CodeGraph README, "Initialize Projects"). Allow `mcp__codegraph` in the permission rules the guard sets, **including the docs-only planner agents** (round 38: their `--allowedTools` and settings).
- **Antigravity:** the same server in the agent's `mcp_config.json` (direct: the user's config is not touched, use the run's own config the way the colony-web server is added; sandboxed: the sandbox's config copy, `sandbox.go` `clientMounts`).
- **Sandboxed runs:** the CodeGraph bundle (`~/.codegraph/current`, its vendored Node) is bind-mounted **read-only** into the sandbox, and the server runs inside with the agent's view of the filesystem. Set `CODEGRAPH_NO_DAEMON=1` there (no background daemon, no file watcher outside the run; connect-time catch-up keeps the index fresh). **Find out and test** whether it can read an index in a folder granted read-only (SQLite WAL wants to write `-shm`); if it cannot, give such folders no CodeGraph and say so in the report rather than widening the grant. No network is needed.
- **Tests:** the MCP config for each client and mode; the sandbox mounts are read-only and nothing outside the grants is reachable (extend the sandbox tests); the docs-only rules allow `mcp__codegraph` and still refuse non-`.md` writes.

## 4. Telling the agents how to use it

- A short section in the shared agent rules (round 14, the rules page), not a hard rule: for "how does X work / where is Y / what breaks if I change Z", call `codegraph_explore` **once**, then read only the exact lines it points to; prefer it over a series of `grep` and whole-file reads; pass a narrow question.
- **Context warning** (from CodeGraph's own measurements): one large answer stays in the context window. Keep questions narrow, and do not repeat an `explore` whose answer is already in the session.

## 5. The colleague's guard

- Update `docs/10-kollega-beallitasa.md`: install CodeGraph for the colleague's Linux user with the reviewed `install.sh` (the same version), turn telemetry off. No `codegraph install`.

## Out of scope

Putting code into the RAG database (round 36 does chat, wiki, decisions and summaries first; code comes later, using CodeGraph's symbol boundaries for chunking). Upgrading CodeGraph automatically (`codegraph upgrade` stays a manual step, see the report).

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (the wiring, indexing, sandbox), `docs/06-allapot-es-uzemeltetes.md` (where the indexes are, how to re-index, how to upgrade CodeGraph on the three machines), `task.md`.

## Report

`docs/handoff/39-codegraph-report.md` (Hungarian): the commits; the read-only grant finding (part 3); a real run on the PC with one agent using `codegraph_explore` (tool calls and tokens of that run from the Runs page, compared with a similar earlier run of the same agent, if there is one); the deploy order (server for the report fields, then all guards); what the reviewer should look at first (the sandbox mounts, the commit filter, the indexing limits).
