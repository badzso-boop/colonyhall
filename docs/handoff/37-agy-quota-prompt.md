# Handoff prompt 37: the guard recognises an Antigravity quota error and pauses instead of failing

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/agy-quota ../new-chat-worktrees/agy-quota main
cd ../new-chat-worktrees/agy-quota
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/37-agy-quota-prompt.md)"
```

---

Branch: `feature/agy-quota`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/agy-quota` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on this branch; never merge, never push. Other rounds (the PWA rounds) run in parallel in their own worktrees; touch only `internal/guard` and what this prompt names, and take the next free migration number if you need one (you should not).

After each part: `gofmt`, `make test`, `make lint`; at the end `go test -race ./internal/guard/ ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it.

## Why (incident, 2026-11-01)

- **The runs:** `@personal-agy` (PC, Antigravity, `gemini-3.8-flash-high`) failed twice, at 09:14 and 09:16.
- **What the guard saw:** `agy: run status "ERROR": `, with an **empty** message. It counted each failure toward `maxAttempts` (`guard.go` `giveUpAfterRetries`), and Norbi did not know why the agent stopped.
- **The real reason** is only in agy's own log (`~/.gemini/antigravity-cli/log/cli-20261101_091403.log`):

  ```
  E1101 09:14:12.844296 errorreport.go:224] agent executor error: generating and executing: RESOURCE_EXHAUSTED (code 429): Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 6h43m54s.
  E1101 09:14:12.875254 session.go:280] Print mode: run ended with error and no response: Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 6h43m54s.
  ```

- **Lines that are noise:** "You are not logged into Antigravity" and "403 … insufficient authentication scopes" appear at the start of every run, successful ones too. They are not errors.
- **The limit is per model:** at the same time a fresh short request worked with the default model and later with `gemini-3.8-flash-high`. So it may be tied to the model and to how large the session is. Do not assume the whole account is blocked.

Round 27 built this for Claude Code (`internal/guard/ratelimit.go`, `ClaudeLimiter`): the guard pauses, does not count attempts, posts one silent note per thread, resumes at the reset time, and shows the state on the Machines page. Antigravity was out of scope there, because agy reports no usage event.

## 1. Read the error agy actually gives

- **Where the message comes from:** `internal/guard/antigravity.go` ~303/411 builds the error from `out.Response`, which is empty on this failure.
  - First find out whether the stream-json `result` event carries the reason in another field (e.g. `error`, `error_message`). Do not guess: a quota failure cannot be triggered on demand, so read agy's printing code paths, or check the `result` of any failing run you can produce (e.g. an invalid model name).
  - If the stream has no reason, fall back to agy's log: the newest `cli-*.log` written during the run in the run's agy home (`~/.gemini/antigravity-cli/log/` for direct runs; the sandbox's config copy for sandboxed runs, see `sandbox.go` `clientMounts`). Take only the `session.go … run ended with error and no response: <message>` line (or the `RESOURCE_EXHAUSTED` line).
  - Read the file with a size cap (the logs can be large: read the tail only), never follow symlinks out of the expected folder, and never log or post anything else from it.
- **Use the message:** put it into the run error, clipped, so the guard log and the hidden error detail (`[error:<id>]`, humans only) say what happened. The chat still gets only "I ran into an error" + the id, except in the quota case below.
- **Tests:** a fixture `result` event with status `ERROR` and the reason (in whichever field you found); a fixture log directory with a realistic log (the noise lines above plus the `session.go` line); the error text contains "Individual quota reached"; an unrelated older log file is not read.

## 2. Pause on an agy quota error, like the Claude limit

- **Recognise it:** `RESOURCE_EXHAUSTED`, `code 429`, `quota reached`; parse "Resets in 6h43m54s" (hours, minutes, seconds; any part may be missing).
  - **Only for a failed run's error, never the answer text.** Round 27 had exactly this bug, where an answer that mentioned "rate limit" paused everything.
- **Scope of the pause:** per **model** on the machine: the key is the agy account (the machine) + the model.
  - Other models on the same machine keep working.
  - Claude Code agents are not affected.
  - Reuse the `ClaudeLimiter` ideas (state file in the guard's state dir, survives a restart, timer to resume, `Wake()`), but generalise it or add an `AgyLimiter`; explain the choice.
- **While paused** (direct, work and decide runs of agy agents using that model):
  - no new run;
  - items stay pending, no ack, attempts unchanged;
  - one silent note per thread: "⏸ Az Antigravity kerete (gemini-3.8-flash-high) elfogyott, 15:58-kor folytatom." (the existing silent rate-limit note endpoint; extend its payload with a client/model label if needed, contract first).
- **Resume** at the reset time + 1 minute. If the message has no reset time, retry after 30 minutes.
- **Report it** in the host heartbeat next to the Claude limits (contract first). The Machines page shows "Antigravity gemini-3.8-flash-high: ⏸ 15:58-ig".
- **Tests:**
  - a quota failure does not count toward `maxAttempts` (fails today);
  - the item stays pending and the note is posted once;
  - the next tick does not run until the manual clock passes the reset;
  - another model on the same machine still runs;
  - a successful answer containing "quota reached" does not pause;
  - restart keeps the pause.

## Out of scope (say so in the report)

Measuring agy's remaining quota before it runs out (agy has no usage event); switching an agent to another model automatically.

## Docs

Hungarian:
- `docs/02-muszaki-specifikacio.md`: where the agy error is read from, and the per-model pause.
- `docs/06-allapot-es-uzemeltetes.md`: what Norbi sees when the agy quota runs out, and where agy's own log is.
- `task.md`: move the item to "Kész".

Then the report `docs/handoff/37-agy-quota-report.md` (Hungarian) with:
- the commits;
- where the reason was found (stream field or log), with the evidence;
- the limiter choice;
- the test outputs;
- what the reviewer should look at first (the log reading: size cap, no symlinks, nothing else leaks).
