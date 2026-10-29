# Handoff prompt 27: answers to an agent's own request reach the thread that asked; the guard respects the Claude usage limit

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/reply-routing ../new-chat-worktrees/reply-routing main
cd ../new-chat-worktrees/reply-routing
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/27-reply-routing-rate-limit-prompt.md)"
```

---

Branch: `feature/reply-routing`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/reply-routing` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/reply-routing`; never merge, never push.

After each part: `gofmt`, `make test`, `make lint`, `make test-web` if the page changed, `make gen-check` if the contract or SQL changed; at the end `make test-pg` uncached (`GOFLAGS=-count=1`) and `go test -race ./internal/guard/ ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it. Check every UI change in a real browser (no `pageerror`).

## 1. The answer to an agent's request goes to a thread that knows nothing about it (bug)

**Incident (production, 2026-10-28):**
1. In `#visegradevent` Norbi asked `@visegrad-claude` to code a feature and to send test runs to `@visegrad-agy` by DM.
2. The run in that channel thread (session `6c541dc5…`, 20:27–20:33) sent the DM, wrote "waiting for the test result" into its note, and ended.
3. `@visegrad-agy` answered "30/30 zöld" in the thread of that DM at 20:34:34.
4. The guard woke `@visegrad-claude` at 20:34:48, but in a **new session for the DM thread** (`65342d8e…`). That session had no history of the work, and its `note_get` was empty because notes are per thread. Following the ack rule it answered only "Rendben, köszi".
5. The next run in the channel (20:36–20:53) told Norbi that "the test result has not arrived yet".

**Root cause:**
- One thread is one CLI session. `ThreadRoot` (`internal/guard/direct.go` ~22) keys the session by the DM thread root, and `DirectSessionID` derives the session from that key.
- The notes are per thread (`internal/modules/notes/mcptools/tools.go`, `resolveThreadID` uses `X-Colonyhall-Thread`).
- Nothing records which thread a request was sent from. The MCP `send` tool (`internal/modules/conversation/mcptools/tools.go`) does not look at the run's `X-Colonyhall-Channel` / `X-Colonyhall-Thread` headers, which the guard already sets (`direct.go` ~516).

**Decision (Norbi, 2026-10-29):** the answer must reach the session that asked. The DM shows where the request came from.

**Fix:**
- **Server: remember the origin.**
  - When an **agent** posts through MCP `send`, store the run's origin on the message: `origin_channel_id` and `origin_thread_id`. They come from the request headers; `Register` needs the `*http.Request` the way the notes tools get it. Store them only when the target differs from the origin.
  - Ignore the origin if the agent is not a member of that channel or the thread does not exist.
  - New migration `00035`, nullable columns, expand pattern. Contract first in `api/openapi.yaml` (message: `origin` with channel id, channel name, thread id).
  - Humans and the REST API never set it.
- **Server: mark the reply.**
  - When a message is a reply to a message whose author is the recipient agent and that carries an origin, the recipient's inbox item gets `reply_origin {channel_id, channel, thread_root_id, request_message_id}`.
  - Two reply shapes must both get it: a reply **in the thread of the request**, and a **top-level reply in the same DM**. For the second, use the recipient's latest origin-bearing top-level message in that DM, and only if the recipient has not posted another top-level message after it.
  - Do not set `reply_origin` if the origin message is retracted (`deleted_at`) or the agent lost access to the origin channel. The item then behaves as today.
  - Delegation and grants are unchanged: approval, `delegated_by` and `via_grant` still work on the DM conversation. Only the routing changes.
- **Guard: run it in the origin's session.**
  - An item with `reply_origin` is grouped and run under the **origin thread key**: same `sessionKey`/session id and same generation as the origin thread.
  - The answer, the progress steps, the run report and `X-Colonyhall-Channel`/`X-Colonyhall-Thread` all use the origin channel and thread. So `note_get` returns the origin's note, and Norbi sees the continuation where he asked.
  - In the DM the guard only adds a 👍 reaction to the reply (wakes nobody); it posts no text there.
  - Prompt framing: keep `<colonyhall_message from=… channel="#dm-…">`, add `answers_your_request="<request id>"`, and add one sentence: "This answers the request you sent from this thread; record the result and continue the task here."
- **Rules:**
  - In `internal/modules/agentrules/model.go` (default rules), `internal/mcpserver/mcpserver.go` and `docs/01-terv.md`, add: "A reply to a request you sent is never only acknowledged: record the result (note_set) and continue the task."
  - The live rules are published by the reviewer after the deploy; do not touch production.
- **UI (`internal/httpserver/web/index.html`):**
  - An agent message with an origin shows a small "↩ a #visegradevent szálból" link under it, which opens that thread.
  - The answer posted in the origin thread from such a run shows "↪ válasz @visegrad-agy DM-jére", linking to the DM reply.
  - Strings via I18N (hu, en).
- **Tests (each fails on today's code):**
  - e2e, reproducing the incident: an agent in channel thread T sends a DM to another agent with the run headers; the other agent replies in the DM thread. Then:
    - the first agent's item carries `reply_origin` pointing to T;
    - the guard (fake stream runner) runs it with the session id of T and posts the answer into T, not into the DM;
    - the run's MCP config carries `X-Colonyhall-Thread: T`.
  - The same for a top-level DM reply.
  - No `reply_origin` when:
    - a human sent the request;
    - the origin message is retracted;
    - the agent left the origin channel;
    - the agent posted a newer top-level DM message without an origin.
  - An agent cannot forge an origin in a channel it is not a member of.
  - Store tests on memory and postgres (`internal/storetest`).

## 2. The guard respects the Claude 5-hour and weekly usage limit (feature)

**Why:** on 2026-10-28 at 12:58 `@visegrad-claude` hit "You've hit your session limit · resets 1pm". The guard counts that as an ordinary failure: `giveUpAfterRetries`, `maxAttempts = 3` (`internal/guard/guard.go` ~126, ~415). Norbi only saw that the agent stopped answering.

**Finding:** Claude Code's stream-json emits a `rate_limit_event`. A probe on 2026-10-28 returned `rate_limit_info.unifiedWindows.five_hour` and `.seven_day`, each `{utilization, resetsAt (unix seconds), status}`. **Capture a real fixture first** (`claude -p "hi" --output-format stream-json --verbose`, keep only that line) and parse exactly what it contains. Fields may be missing: do not guess, ignore what is absent.

**Defaults:** both are `guard.yaml` keys. The reviewer confirms the values with Norbi before merging.
- `rate_limit_reserve: 90`: at or above 90% utilization of either window the guard **starts no new Claude Code run**. A running run is never killed.
- `rate_limit_warn: 80`: the page shows a warning colour. This is UI only and changes no behaviour.

**Fix:**
- **Parse** the event in `claudecode.go` (next to the `thinking`/`text`/`tool_use` cases ~282) into a `RateLimit` value on the run result and the stream callback.
- **Keep the state per guard.** One Claude account per machine, shared by all its Claude Code agents. Persist it in the guard's state dir so a restart keeps it, and log changes (`guard: claude usage`, utilization, resets, never secrets).
- **Before any Claude Code run** (direct, work, and the cheap `Decide` wake-up): if either window is at or above the reserve, or `status` is not allowed, do not start.
  - The items stay pending: no ack, and **no attempt counted**.
  - Post one silent note per thread (wakes nobody, the existing `silent` path from round 26): "⏸ A Claude 5 órás kerete 91%-on, 18:00-kor folytatom." For the weekly window the text names the day.
  - Then resume automatically after `resetsAt` (+1 min) with a timer, not polling. Also resume if a newer event shows the utilization back under the reserve.
- **A run that fails with the limit message** ("hit your session limit", "usage limit"; match loosely and keep the patterns in one place) is not an attempt and gets no error message. Treat it like the reserve case:
  - use the reset time from the latest event, or parse it from the message ("resets 1pm", Europe/Budapest);
  - otherwise retry after 15 min.
- **Report to the server** in the guard's existing host heartbeat/status (contract first): per machine, the windows (utilization, resets_at, status) and whether runs are paused.
  - The "Gépek" panel and the agent overview show "Claude 5 óra: 72% · újraindul 18:00 · heti 59%". The warning colour applies at or above `rate_limit_warn`, a "⏸ szünetel" badge at or above the reserve.
  - Strings via I18N.
- **Antigravity is out of scope:** agy reports no such event.
- **Tests:**
  - parsing on the real fixture plus missing fields;
  - at or above the reserve: no run, items not acked, attempts unchanged, the note posted once per thread, and the run starts after the manual clock passes `resetsAt`;
  - a limit-message failure is not counted toward `maxAttempts` (fails today: 3 limit failures give up);
  - restart keeps the pause;
  - below the reserve nothing changes;
  - the heartbeat carries the state and the page renders it (browser check).

## Out of scope (say so in the report)

Antigravity limits, a reserve per agent, a manual "run anyway" override.

## Docs

Hungarian:
- `docs/01-terv.md`: answer routing to the origin thread; usage limit, reserve and warning.
- `docs/02-muszaki-specifikacio.md`: the origin columns and `reply_origin`; the session-key rule; `rate_limit_event`; the heartbeat fields.
- `docs/06-allapot-es-uzemeltetes.md`: the `guard.yaml` keys.
- `guard.example.yaml`: `rate_limit_reserve`, `rate_limit_warn`.
- `task.md`: move the item to "Kész" with the date and commit ids.

Then the report `docs/handoff/27-reply-routing-rate-limit-report.md` (Hungarian) with:
- the commits and the decisions where this prompt was open (how a top-level DM reply finds its request; where the limit state lives);
- the outputs of `make test`, `make lint`, `make test-web`, `make test-pg` (uncached), `make gen-check` and the `-race` run;
- a manual check on the dev stack: a fake agent asks another one by DM from a channel thread, and the answer continues in that thread; the limit pause with a fixture event;
- what the reviewer should look at first.
