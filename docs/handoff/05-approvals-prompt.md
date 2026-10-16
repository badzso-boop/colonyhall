# Handoff prompt 05: approvals from the chat (for Antigravity CLI)

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/05-approvals-prompt.md)"
```

---

You are continuing the Go project **Colonyhall** in the current directory. Claude Code designed it and reviews your work afterwards. This task touches authorization: be strict, and test every "who may do this" rule.

## Get the rhythm (read before coding)

1. `CLAUDE.md` (binding rules, commands, schedule).
2. `docs/handoff/01-direct-mode-prompt.md` section 2 **"Ground rules"** — all apply (new branch from `main`, no commits to `main`, no push, **no real `claude`/`agy` calls**, contract-first, `make gen`, storetest memory+Postgres, e2e contract tests, per-step checks, `make test-pg` + `make gen-check` at the end, one commit per step, no AI attribution).
3. `docs/01-terv.md`: "Működési modell" (especially **"Ki adhat feladatot kinek"**) and the paragraph **"Jóváhagyás a chatből (terv, 2026-10-16)"**.
4. Code: `internal/mcpserver/`, `internal/modules/conversation/mcptools/tools.go` (how MCP tools are registered and how the current actor is re-checked on every call), `internal/modules/runs/` (the `[error:<id>]` pattern: details for humans only, redaction with `runs.Redact`, access needs channel read rights — copy this pattern), `internal/guard/direct.go` (`writeMCPConfig`, the Claude `Direct` arguments), `internal/modules/hosts/` (assignment fields), `internal/httpserver/web/index.html` (the error details button/dialog).

Branch: `git switch -c feature/approvals`.

## How Claude Code's permission prompt tool works

With `claude -p … --permission-prompt-tool mcp__colonyhall__approve`, whenever Claude Code would ask the user for permission (in `auto` mode: what the classifier does not allow by itself), it calls that MCP tool with a JSON input that contains `tool_name` and `input` (the tool's arguments; there may be more fields — accept and ignore unknown ones). The tool must answer with a single text content holding JSON: `{"behavior":"allow","updatedInput":<the input, unchanged>}` or `{"behavior":"deny","message":"<why>"}`. The call blocks Claude until it returns; set the MCP tool timeout for the run with the environment variable `MCP_TOOL_TIMEOUT` (milliseconds) a bit above the approval timeout.

## Task

### Server (new module `approval`, layered like the others)

- Migration: append-only `approval_request` (`id`, `workspace_id`, `agent_id`, `channel_id`, `thread_root_id` nullable, `tool_name`, `input` (redacted, ≤ 8000 chars), `state` (`pending` | `allowed` | `denied` | `expired`), `decided_by` nullable, `decision_message`, `created_at`, `decided_at`, `expires_at`) — a decision is an UPDATE of a pending row only (conditional), never a delete.
- MCP tool `approve` (registered for agents): reads the run's channel/thread from the MCP request headers `X-Colonyhall-Channel` and `X-Colonyhall-Thread` (the guard sets them, see below); the agent must be able to read that channel. It stores the request, posts into the thread **as the agent** a short message `🔐 Approval needed: <tool_name> [approval:<id>]` (no input in the chat), then waits for a decision until `expires_at` (default 10 min; wait on the bus or poll the store every second — cancellable by ctx), and answers Claude with allow/deny JSON. Expired → deny with message "no answer in time".
- Halt: if the agent is paused/stopped or the emergency stop is on while waiting, answer deny immediately and mark the request `denied` ("agent halted"). The emergency stop also expires all pending requests of the workspace.
- REST: `GET /api/v1/approvals/{id}` (humans who can read the channel; agents 403) returns tool name, redacted input, state; `POST /api/v1/approvals/{id}` `{"decision":"allow"|"deny","message"?}` — humans only, must be able to read the channel, the requesting agent must not be able to decide (it is not human anyway — test it), only a pending request can be decided (second decision → 409). Events `approval.requested` / `approval.decided` (no input in the payload) for the audit log.
- OpenAPI first, then `make gen`; storetest; e2e contract tests.

### Assignment + guard

- `agent_host` gets `approvals` (`none` | `chat`, default `none`), in OpenAPI, hosts API, dev page assign form. Only meaningful for `claude-code` direct mode (validate: `chat` requires client `claude-code`).
- Guard, Claude direct runs with `approvals=chat`: add `--permission-prompt-tool mcp__colonyhall__approve`, set `MCP_TOOL_TIMEOUT` for the process (approval timeout + 60 s), and add the headers `X-Colonyhall-Channel` / `X-Colonyhall-Thread` of the run's thread to the temporary MCP config (in `writeMCPConfig`). The direct run's own timeout must be long enough to include waiting for an approval.

### Dev page

Render `[approval:<id>]` (like `[error:<id>]`): for humans a "Részletek" button showing tool name and input, plus "Engedélyezem" / "Elutasítom" while pending; show the final state once decided (refresh on the `approval.decided` event).

## Tests (no real CLIs)

- e2e through the real MCP endpoint (see the existing MCP e2e tests for how to open a session with a token and custom headers): approve flow (request → message in the thread with the reference → human allows → tool returns allow JSON with the input), deny flow, expiry (manual clock or short timeout), agent cannot read or decide, a human who cannot read the channel cannot read or decide, second decision 409, emergency stop during the wait → deny, secrets in the input are redacted.
- Unit: Claude direct args with/without approvals; MCP config headers present; env var set.
- `-race` on guard and e2e.

## Docs

`docs/02-muszaki-specifikacio.md`: a "Jóváhagyás a chatből (megvalósítva)" paragraph (table, tool, endpoints, timeouts); `CLAUDE.md` schedule. Hungarian.

## Report

Write `docs/handoff/05-approvals-report.md` (Hungarian) as before, commit, print a 5-line summary, stop. Do not merge.
