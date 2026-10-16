# Handoff prompt 04: run log and token usage on the server (for Antigravity CLI)

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/04-run-log-prompt.md)"
```

---

You are continuing the Go project **Colonyhall** in the current directory. Claude Code designed it and reviews your work afterwards.

## Get the rhythm (read before coding)

1. `CLAUDE.md` (binding rules, commands, schedule).
2. `docs/handoff/01-direct-mode-prompt.md` section 2 **"Ground rules"** — all apply (new branch from `main`, no commits to `main`, no push, **no real `claude`/`agy` calls**, contract-first, `make gen`, storetest memory+Postgres, e2e contract tests, `gofmt`/`make test`/`make lint` per step, `make test-pg` + `make gen-check` at the end, one commit per step, no AI attribution).
3. `docs/01-terv.md`: paragraph **"Futásnapló és tokenfelhasználás (terv, 2026-10-16)"** and section "Keretek és költségkorlátok" (future budget limits will build on this; do not implement limits now).
4. Code: `internal/modules/runs/` (it already stores run **errors**; extend this module), `internal/guard/guard.go` (`logRun`), `internal/guard/direct.go` (`logDirectRun`), `internal/guard/client.go`, `migrations/`, `internal/storetest/`, `internal/httpserver/web/index.html` (dev page; follow how the "Gépek" and ⏰ panels are built, always `textContent`, never `innerHTML` with data).

Branch: `git switch -c feature/run-log`.

## Task

### Server (`runs` module)

- Migration: append-only table `agent_run`: `id`, `workspace_id`, `agent_id`, `host_id` (nullable), `phase` (`decide` | `work` | `direct`), `client`, `model`, `session_id`, `channel_id` (nullable), `thread_root_id` (nullable), `items` (int), `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens` (bigint), `duration_ms` (int), `status` (`ok` | `error` | `halted`), `error_id` (nullable, references `run_error`), `created_at`. Index on `(workspace_id, agent_id, created_at)`.
- `POST /api/v1/agent/runs` (agent token): the calling agent reports one run. Validate enums, non-negative numbers, sane maxima (e.g. tokens ≤ 10^9, duration ≤ 24 h); `agent_id` is always the caller (never from the body). Event `run.reported` (summary payload) for the audit log.
- `GET /api/v1/runs?agent_id=&limit=` — humans only; newest first; limit ≤ 200.
- `GET /api/v1/usage?days=7` — humans only; per agent per day (in `Europe/Budapest`) sums of tokens, run counts and error counts. Days ≤ 90.
- OpenAPI first, then `make gen`. Store (memory + postgres via sqlc) + storetest, e2e contract tests (including: an agent cannot read runs/usage; an agent cannot report a run for another agent).

### Guard

- After every run (decide, work, direct) report it with `Client.ReportRun` (best effort: a failure is logged, never fails the run). Include the error id when the run produced one (`reportError` returns it — adjust as needed).
- Keep the existing log lines.

### Dev page

A 📊 button (humans) opening a panel: a table per agent with today's and the last 7 days' tokens (in/out/total), runs and errors, and the last 20 runs (time, agent, phase, model, tokens, duration, status; a run with an error id gets the existing "error details" button).

## Tests (no real CLIs)

- storetest for the new store methods (memory + Postgres), including the per-day aggregation boundary at Budapest midnight.
- e2e: a scripted guard agent run produces a run record with the right tokens; usage sums; humans only; validation errors return 400.
- `-race` on guard and e2e.

## Docs

Mark it implemented in `docs/02-muszaki-specifikacio.md` (new short "Futásnapló" paragraph in the Őr section: table, endpoints) and `CLAUDE.md` schedule. Hungarian.

## Report

Write `docs/handoff/04-run-log-report.md` (Hungarian) as in the earlier reports, commit, print a 5-line summary, stop. Do not merge.
