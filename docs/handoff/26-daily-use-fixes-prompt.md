# Handoff prompt 26: fixes from a day of real use (live steps, long runs, terminals, mentions, replies, delegation questions)

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/daily-use-fixes ../new-chat-worktrees/daily-use-fixes main
cd ../new-chat-worktrees/daily-use-fixes
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/26-daily-use-fixes-prompt.md)"
```

---

Branch: `feature/daily-use-fixes`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/daily-use-fixes` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/daily-use-fixes`; never merge, never push.

## Why

Norbi used Colonyhall all day on 2026-10-27 with agents on three machines. The reviewer checked each complaint against the code and the production logs; the findings are below with file references. Fix them in the order given; one commit (or a few) per part.

After each part: `gofmt`, `make test`, `make lint`, `make test-web` if the page changed, `make gen-check` if the contract or SQL changed; at the end `make test-pg` uncached (`GOFLAGS=-count=1`; if the server image does not build here, start only the Postgres container) and `go test -race ./internal/guard/ ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it. **Run the dev page in a real browser for every UI part** (Playwright or by hand) and make sure there is no `pageerror` — round 25 shipped an undefined variable that no test caught.

## 1. Live steps arrive only at the end (bug)

**Finding:** the adapters are built with `Run` only (`internal/guard/adapter.go`, `Adapters`: `&ClaudeCode{Bin: cfg.Command, Run: run}`, same for Antigravity). With `StreamRun == nil`, `execStream` (`claudecode.go` ~331, `antigravity.go` ~250) wraps `Run`, which collects the **whole** stdout and replays the lines after the process exits. So every step ("📖 olvas…", "⚙️ parancs…", "Válaszol…") reaches the progress reporter at the end of the run, never while it works.

**Fix:**
- Give the adapters a real `StreamRunner` in production (through the factory, so `guard.go` ~1018 and `host.go` ~423 both get it); keep `Run` for tests and for the non-streaming calls.
- `ExecStreamRunner` must behave like `ExecRunner`: process group, kill the whole group when ctx ends, and **not hang when a child keeps stdout open** after the client exits (e.g. the agent started a dev server in the background): set `cmd.WaitDelay`, and read stdout so that `Wait` cannot block forever. Lines can be long (MB-sized JSON lines exist, see `TestStreamHugeLine`).
- Make sure a "thinking" phase is shown too (Claude Code stream-json thinking blocks; agy `agent_response` ACTIVE): the user wants to see *now it reads a file, now it thinks*, live.
- Test: a fake `StreamRunner` that emits lines with pauses; assert that `OnStep` is called (and the progress reporter sends) **before** the run returns. Test the hanging-child case with a real short shell command that leaves a background child holding stdout.

## 2. Long runs are killed after 15 minutes (bug)

**Finding:** direct runs get a hard deadline `work_timeout` (default 15 min, `config.go` ~172, `direct.go` ~613). A big task is killed mid-work (`context deadline exceeded`; on 2026-10-27 `@nato-claude-opus` at 22:03 and 22:20, `@szakdoga-claude` at 07:21), then retried up to 3 times from the same session, which looks like the agent "waiting for the continuation" while it should be working.

**Decision (Norbi): a working agent must not be stopped by a clock.**
- Replace the fixed deadline for direct and work runs with an **inactivity timeout**: kill only when the client produced **no stream output** for `idle_timeout` (default 15 min, `guard.yaml`), plus a generous hard cap `max_run` (default 4 h, `guard.yaml`) as a last resort. This depends on part 1 (output arrives live).
- When the guard kills a run for inactivity or the cap: **no automatic retry** of the same items (a retry restarts the heavy work); post one short message in the thread, without internal details: "⏱ Leállítottam: 15 perce nem jött semmi a klienstől." / "⏱ Leállítottam: elérte a 4 órás korlátot." and log the details. The items are acked.
- The "dolgozik…" indicator keeps running while the run is alive.
- Tests (manual clock / fake stream runner): a run that keeps emitting lines for longer than `idle_timeout` is not killed; a silent run is killed after `idle_timeout`; the cap kills; no retry after either; the message is posted once.

## 3. Terminals cannot be found when they start (UX)

**Finding:** terminals work (on 2026-10-27 `@szakdoga-claude` started `sim`, `keszenlet`, `colcon-build`, `regresszio`), but the page shows a "🖥 Terminál megtekintése" button only under the **finish** message (`index.html`, the `termMatch` regex for `kész|hibával állt le|leállítva`), and in the agent overview panel. Nothing appears in the chat when a terminal starts. The terminal code also logs nothing to the guard log.

**Fix:**
- On `term_start`, the guard posts one short message in the thread: "🖥 `<name>` elindult: `<command>`" (command redacted with the existing `Redact`, clipped) with the same "Terminál megtekintése" button. This message must **wake nobody** (see the `silent` send option in part 6) — in an agent–agent DM it would otherwise wake the other agent.
- While an agent has running terminals, show it next to its "dolgozik…" / in the channel header: "🖥 2 terminál fut" → click opens the list (reuse the overview panel's list and `openTerminalViewer`).
- Log start, finish (exit code, duration) and kill in the guard log (`guard: terminal started` …), never the output.
- Tests: the start message is posted once, wakes nobody (no inbox row for anyone); the UI button appears on the start message (browser check).

## 4. `@` mention autocomplete (feature)

**Finding:** the composer has no suggestions; the full handle must be typed.

**Fix:** typing `@` and letters in any composer (channel, thread, DM) opens a small list: channel members first, then everyone else (humans and agents, not halted ones), **fuzzy** matched on handle and display name (subsequence match, prefix and word-start hits ranked first), at most 8 rows, with the agent/human marker and the machine name for agents. ↑/↓ moves, Enter/Tab inserts `@handle `, Esc closes, click works, works on a phone (no hover needed). The list never hides what is being typed. Strings via I18N (hu, en). Unit-test the ranking function (`node --test`, next to `markdown.test.mjs`); browser-check the widget.

## 5. Reply does not always wake the agent (UX)

**Finding:** "Válasz" only opens the thread (`setReply`). A human's reply in a channel thread wakes the agents that already wrote in that thread (`delivery.ThreadFollowers`), but **only when the reply mentions nobody** — any `@` in it reaches only the mentioned actors — and only agents that wrote in that thread. No other failure was found in the code; Norbi's proposal makes it explicit.

**Decision (Norbi):** clicking "Válasz" on a message written by an agent (not by me) **pre-fills `@<that agent> `** in the composer, cursor after it; the user can delete it. On a human's message nothing is pre-filled. Browser-check it; no server change.

## 6. Delegation questions: one place, no extra wake (bug + decision)

**Finding:** for an agent→full-access-agent task (round 24), the page shows "Jóváhagyom / Elutasítom" under the **original agent message** (`isDelTarget`) *and* the server posts a separate "Q-NN (to humans): … Options: …" message in the thread (`question.Service.Ask` always posts), then a "Q-NN answer: Jóváhagyom" message. In an agent–agent DM every one of these messages wakes **both** agents (DM recipients get `FlagDM|FlagWake`, `delivery.FindRecipients`) — the source agent is woken for nothing.

**Decision (Norbi, 2026-10-28):** the buttons under the agent's message are enough. For **delegation** questions:
- Do **not** post the "Q-NN (to humans)" message and do **not** post the "Q-NN answer" message. The question still exists (number, deadline, events, "Válaszra vár" tab, inline buttons and the decided state "✅ Jóváhagyta @norbi …" under the original message, as round 25 renders it).
- On approve: only the target is woken (the `delegated` item). On deny/expire: the source agent gets its answer item as today; the "elutasította"/"lejárt" note, if posted at all, must wake nobody else.
- Add a server-internal **`silent`** send option (no inbox rows, no wake for anyone) for such bookkeeping messages and the terminal start message; it is **not** settable through the public API or MCP by agents (an agent must not be able to post silently to dodge rate limits or approvals). Decide where it lives (e.g. `conversation.SendInput.Silent`, honoured by `delivery.FindRecipients`) and explain it in the report.
- Questions agents ask humans with `ask` (not delegations) keep their "Q-NN" message: that message *is* the question.
- Tests: an agent–agent DM delegation creates the question with **no new chat message**; approving wakes only the target (exactly one `delegated` item, nothing for the source); denying gives the source one answer item and the target nothing; the "Válaszra vár" list still shows it; agents cannot set `silent` (API/MCP ignore or reject it).

## 7. A grant does not cover the thread of the request in a DM (bug, round 25)

**Finding (production, 2026-10-27, Q-23 → Q-25):** `@visegrad-agy` asked `@agy-homelab` at the top level of their DM; the approval created a grant for the conversation `(DM, thread_root = nil)`. The question, the answer and `@agy-homelab`'s report all went into the **thread** of the request (agents reply in the thread), which `MessageConversation` treats as another conversation `(DM, root)` → the report produced a new question (Q-25) although the grant was active.

**Fix:** in a DM the conversation is **the whole DM, any thread** (`MessageConversation` returns `thread_root = nil` for every DM message). Channels stay as they are. Existing grants in production already use `(DM, nil)`, so no data change. Update the table test, and an e2e test that reproduces the incident: top-level request in a DM, approval, the target replies in the request's thread → no new question, the source gets a `delegated` item with `via_grant`.

## Out of scope (say so in the report)

Interactive browser terminals (decision 2026-10-26: SSH only), mention autocomplete for channels (`#`), changing who may approve.

## Docs

Hungarian: `docs/01-terv.md` (timeouts: inactivity + cap; terminal start message; mention autocomplete; reply pre-fill; delegation questions without chat messages; DM grant scope), `docs/02-muszaki-specifikacio.md` (`StreamRunner` in production, `idle_timeout`/`max_run`, the `silent` send option and who may set it, the DM conversation rule), `docs/06-allapot-es-uzemeltetes.md` (the table rows and `guard.yaml` keys), `guard.example.yaml` (`idle_timeout`, `max_run`), `task.md` (move the item to "Kész" with the date and commit ids). Then the report `docs/handoff/26-daily-use-fixes-report.md` (Hungarian): commits, the decisions where this prompt was open (where `silent` lives; how the stream runner avoids hanging), what is left out, the outputs of `make test`, `make lint`, `make test-web`, `make test-pg` (uncached), `make gen-check`, the `-race` run, a manual check on the dev stack with a browser (live steps appear while a fake agent runs; a terminal start message with its button; mention list with fuzzy matches; reply pre-fill; an agent–agent DM delegation with no extra messages), and what the reviewer should look at first.
