# Handoff prompt 22: live progress of agent runs (what the agent is doing, step by step)

**Start after round 21 (`feature/compaction`) is reviewed and merged:** both change the guard's run handling (`internal/guard/direct.go`, `guard.go`, the adapters).

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/live-progress ../new-chat-worktrees/live-progress main
cd ../new-chat-worktrees/live-progress
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/22-live-progress-prompt.md)"
```

---

Branch: `feature/live-progress`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/live-progress` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/live-progress`; never merge, never push.

## Why

While an agent works, the chat shows only "@x dolgozik…" — a black box, sometimes for 15 minutes. The guard starts the clients with `--output-format json`, which returns one result at the end. Both clients can stream their steps instead (`--output-format stream-json`), measured on 2026-10-26:

- **Claude Code** (`claude -p … --output-format stream-json --verbose`): JSON lines; `system/init`, `assistant` messages whose content has `tool_use` (`name`: `Read`, `Write`, `Edit`, `Bash`, `Glob`, `Grep`, `WebFetch`, MCP tools; `input` with `file_path` / `command` / `pattern`), `thinking` blocks (no text in headless mode), `text`; `user` messages with `tool_result`; `system/permission_denied`; `system/thinking_tokens`; `rate_limit_event`; a final `result` (the same fields as the `json` output: `result`, `session_id`, `usage`, `num_turns`, `is_error`, `subtype`). **Each `assistant` message carries its own `usage`** — the context of that call.
- **Antigravity** (`agy -p … --output-format stream-json`): JSON lines `{"event": …}`: `init` (`conversation_id`, model, cwd), `step_update` (`step_index`, `state` `ACTIVE`/`DONE`, `step_type` `user_input` / `agent_response` / `tool`; tools with `tool_name` and `tool_info.parameters`, e.g. `read_file` + `Path`; `agent_response` with `text_delta` while writing and, when `DONE`, **its own `usage`** and `duration_seconds`), and a final `result` (`status`, `response`, `usage`, `conversation_id`, `denied_actions`).

So the guard can show each step live, keep the list in the run log, and — a welcome side effect — **measure an Antigravity session's real context** (the last call's input), which today it cannot (round 2026-10-26 switched Antigravity's token-based session rotation off for that reason).

Read first: `CLAUDE.md`, `internal/guard/claudecode.go` and `internal/guard/antigravity.go` (`execAs`, the `json` parsing, the `ERROR`-after-answer rule with `output_tokens > 0`, `Direct`, `Work`, `Decide`), `internal/guard/adapter.go` (`RunResult`, `Usage`, `ContextTokens`, `ClientWarning`), `internal/guard/direct.go` (`reportsContext`, session rotation, `claudeContextTokens` / transcript fallback), `internal/guard/guard.go` (`withActivity`, `logRun`, `reportRun`), the activity indicator path (`internal/gateway/gateway.go` `ReportActivity`, the dev page's activity bar), `internal/modules/runs/` (run reports, the round-19 `web_reads` field as the pattern for an extra list), `internal/server/overview.go`, `internal/httpserver/web/index.html`.

Do the parts in this order; one commit (or a few) per part; after each part `gofmt`, `make test`, `make lint`, `make test-web` if the page changed. `make test-pg` uncached (`GOFLAGS=-count=1`; if the server image does not build here, start only the Postgres container) when a store changes; `go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/`. Contract first. Migrations: the next free numbers. **Commit only when `make test` and `make lint` pass.**

## Decisions (Norbi, 2026-10-26; do not change them)

- **Direct and work runs stream; the short tool-less decision run and the scribe run stay on `json`.**
- **What is shown is a step, never content:** the tool's kind and a short label. Never file contents, tool output/results, thinking text, the prompt, or the answer text while it is being written (only "✍️ válaszol…").
- **Paths** are shown relative to the agent's root folder / worktree / granted folder when inside them; anything outside → only the file name. **Commands** (Bash/shell): the first 80 characters after redaction (below).
- **Only humans who can read the channel** see the steps (like the activity indicator). Agents never get them.
- The step list is **kept with the run** (run log, agent panel, and under the agent's reply), capped at 200 steps per run.

## 1. Guard: streaming adapters and a step model

- `Step{Kind, Label, Path, State, StartedAt, Duration}`; kinds: `read`, `write`, `edit`, `search` (Glob/Grep/find), `command`, `web` (WebFetch/web_read), `mcp` (other MCP tools: label = tool name, e.g. `colonyhall.send`), `think`, `answer`, `denied`, `other`.
- Claude Code `Direct`/`Work`: `--output-format stream-json --verbose`; read stdout **line by line while the process runs** (not after), parse each JSON line, map events to steps, keep the **last assistant message's usage** as `ContextTokens` (input + cache read + cache creation of that call; the transcript reading of `claudeContextTokens` stays as a fallback), and build the `RunResult` from the final `result` line exactly as from today's `json` output (text, session id, usage, errors). A missing `result` line = the same error as an unreadable output today.
- Antigravity `Direct`/`Work`: `--output-format stream-json`; map `step_update` to steps (`tool` → by `tool_name`: `read_file`→read, `write_file`/`replace`/`edit`→write/edit, `run_command`/shell→command, search/grep/list→search, others→other; `agent_response` ACTIVE with `text_delta`→answer (no text), DONE→ ends it); take `ContextTokens` from the **last `agent_response` DONE usage** (its input tokens + cache read); the final `result` builds the `RunResult` as today, **including the `ERROR`-after-answer rule** (`response` non-empty **and** `output_tokens > 0`) and `denied_actions`.
- With that, `reportsContext("antigravity")` becomes true **only when the run streamed a per-call usage** (keep a fallback: no per-call usage → no token-based rotation, as today). Update the comment and the tests of the 2026-10-26 change accordingly.
- **Redaction** (one function, tested): in commands and labels, replace values that look like secrets with `***` — `KEY=value` / `--token value` / `--password …` / `Authorization: …`, URLs with `user:pass@`, `ghp_…`, `sk-…`, `col_…`, `AKIA…`, JWT-like `xxx.yyy.zzz`, long hex/base64 runs (≥ 32 chars); cap 80 characters; strip control characters.
- The adapters report steps through a callback in the request (`OnStep func(Step)`), so the adapter stays free of networking; tests feed recorded stream files (keep the two probe outputs from this prompt's "Why" as fixtures, trimmed, with paths anonymised).
- Tests: Claude and Antigravity fixtures → the expected step list, the same `RunResult` as the `json` path for the same run, `ContextTokens` from the last call; a stream that ends without `result` → error; a huge single line (1 MB) does not break the reader (use a large `bufio.Scanner` buffer or a `bufio.Reader`); redaction cases; `ERROR` after an answer with `output_tokens > 0` kept, with 0 refused.

## 2. Guard → server → page, live

- `POST /api/v1/agent/progress` (agent token): `{channel_id, thread_root_id?, run_key, steps: [Step…], done?: bool}`; the agent must be able to read the channel (403 otherwise). The guard batches: at most one call per second per run, the steps since the last call; best effort (a failure is logged, never fails the run), like the activity reports.
- Server: **not stored here**; it keeps the current run's steps in memory per `run_key` (capped at 200, dropped 2 minutes after `done` or after 15 minutes without an update), and pushes an ephemeral WebSocket frame `agent.progress {agent_id, channel_id, thread_root_id, run_key, steps (the new ones), done}` to connections **that can read the channel** (the same per-recipient `CanRead` check as `agent.activity`). `GET /api/v1/channels/{id}/progress` (humans who can read it) returns the runs in progress with all their steps so far, for a page opened mid-run.
- **Run log:** the guard's run report gets an optional `steps` list (the final list, capped at 200) and `reply_message_id` (the id of the message the guard posted as the answer, when there is one). Store them with the run (contract first; migration; the round-19 `web_reads` field is the pattern). The agent panel's last runs and the "Futások" tab show "12 lépés" with an expandable list.
- Tests: e2e over the contract (progress accepted; agent without channel access 403; a human without access gets no frame; the in-progress endpoint; run report with steps and reply id stored and returned); gateway: frames only to readers; the in-memory store's caps and expiry with the manual clock.

## 3. Dev page

- **While running:** under "@x dolgozik…" (or replacing it) an expandable block per running agent, in the open channel / thread: elapsed time, and the steps as one line each with an icon (📖 read, ✏️ write/edit, 🔎 search, ⚙️ command, 🌐 web, 🧩 mcp, 💭 think, ✍️ answer, ⛔ denied), the active one marked; by default collapsed to the last 3 steps with "összes (14)". A page opened mid-run loads `GET …/progress`.
- **After the run:** under the agent's reply (matched by `reply_message_id`) a collapsed "▸ 14 lépés · 2:14" line that opens the stored list (fetched from the run on demand).
- All strings via I18N (hu, en); desktop and phone (390 px). No step text through `innerHTML`.

## Out of scope (say so in the report)

Showing file contents or diffs of edits, the thinking text, cancelling a single step, streaming the answer text into the chat.

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (the streaming adapters, the step model and redaction, the progress endpoint and frame, the run report fields, the Antigravity context measurement), `docs/01-terv.md` (the "Élő folyamatjelzés" item under "Későbbre hagyott döntések" is now implemented), `task.md` (add as done with the date; keep the commit ids), `CLAUDE.md` schedule. Then the report `docs/handoff/22-live-progress-report.md` (Hungarian): commits, decisions where this prompt was open, what is left out, the outputs of `make test`, `make lint`, `make test-web`, `make gen-check`, `make test-pg` (if run), the `-race` run, a manual check (a real Claude Code and a real Antigravity direct run on the dev stack: the steps seen live, the stored list, and the measured context of the Antigravity run), and what the reviewer should look at first.
