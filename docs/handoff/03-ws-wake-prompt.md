# Handoff prompt 03: WebSocket wake-up for the guard (for Antigravity CLI)

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/03-ws-wake-prompt.md)"
```

---

You are continuing the Go project **Colonyhall** in the current directory. Claude Code designed it and reviews your work afterwards.

## Get the rhythm (read before coding)

1. `CLAUDE.md` (binding rules, commands, current schedule).
2. `docs/handoff/01-direct-mode-prompt.md` section 2 **"Ground rules"** — they all apply here (new branch from `main`, never commit to `main`, never push, **no real `claude`/`agy` calls**, contract-first, `make gen`, storetest for memory+Postgres when a store changes, e2e contract tests, `gofmt`/`make test`/`make lint` after each step, `make test-pg` and `make gen-check` at the end, one commit per step, no AI attribution lines).
3. `docs/01-terv.md`: "Működési modell" and the paragraph **"WebSocket-ébresztés (terv, 2026-10-16)"**.
4. Code: `internal/gateway/gateway.go` (the WebSocket protocol: hello / welcome / event / replay_done / resync_required), `internal/guard/guard.go` (`Agent.Run`, `Tick`, `pause`), `internal/guard/host.go` (`startAgent`, `hostTokens`), `internal/guard/client.go`, `internal/e2e/supervisor_test.go`, `internal/e2e/supervisor_direct_test.go`.

Branch: `git switch -c feature/ws-wake`.

## Task

Today each guard agent loop polls `GET /me` + `GET /agent/inbox?limit=1` every `poll` (5 s). Make it event-driven:

1. Per agent, the guard keeps a WebSocket to `/ws/ui` open with the agent's token in the `hello` frame (`github.com/coder/websocket` is already a dependency). No `last_seq`: start from the head. Any `event` frame (and `resync_required`) signals "check the inbox now". Ignore `welcome`/`replay_done`.
2. The agent loop waits on either that signal or a fallback timer, then runs `Tick` as today (keep the debounce, the batching, the halt checks and the backoff exactly as they are). New config field `fallback_poll` (default 60 s) in `AgentConfig`/`defaults`; keep `poll` for the agents mode default behaviour only if the WebSocket cannot be used.
3. Reconnect with backoff (1 s → 30 s) when the socket closes. In host mode the agent token is short-lived (1 h, renewed by `hostTokens`): reconnect when the token changes, and when the server closes the connection.
4. Never let the WebSocket replace the halt safety: `watchHalt` keeps polling `/me` during runs.
5. Log connect/disconnect at debug level only (no token in logs).

## Tests (no real CLIs)

- e2e: a guard agent with the WebSocket connected reacts to a mention well before the fallback interval (set `fallback_poll` to e.g. 1 minute in the test and assert the run happens within ~2 s after the message is sent and the consumers drained).
- e2e: if the WebSocket is unavailable (e.g. close the connection server-side / invalid token for the WS only via a test hook), the fallback poll still picks the item up.
- Unit: reconnect backoff bounds; token change triggers a reconnect (fake token source).
- Run the guard and e2e packages with `-race` at least twice.

## Docs

Mark the WebSocket wake as implemented in `docs/02-muszaki-specifikacio.md` (Őr section: replace the "5 mp-es lekérdezés" description) and in `CLAUDE.md` ("Eltérések a tervtől": the guard no longer polls every 5 s). Hungarian.

## Report

Write `docs/handoff/03-ws-wake-report.md` (Hungarian): commits, decisions, anything left out, final results of `make test`, `make lint`, `make test-pg`, `make gen-check`, what the reviewer should look at first. Commit it. Print a 5-line summary and stop. Do not merge.
