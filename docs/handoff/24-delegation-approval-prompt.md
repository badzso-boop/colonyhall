# Handoff prompt 24: an agent's task for a full-access agent goes through a human's approval

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/delegation-approval ../new-chat-worktrees/delegation-approval main
cd ../new-chat-worktrees/delegation-approval
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/24-delegation-approval-prompt.md)"
```

---

Branch: `feature/delegation-approval`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/delegation-approval` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/delegation-approval`; never merge, never push.

**Order:** this round touches `internal/guard/direct.go` (`humanOnly`) and the delivery/question modules. If round 21 or 22 is in progress, wait until it is merged; do not run in parallel with them.

## Why

A rule from the start (`CLAUDE.md`, "Feladatadás és hibák"): **only a human may give a full-access agent a task**; an agent may only task sandboxed agents. The guard enforces it in `humanOnly` (`internal/guard/direct.go`): an item from an agent is refused with a short note (`guard: task from a non-human refused`). On 2026-10-26 `@projects` (PC) asked `@agy-homelab` (homelab, full access) to move a project to the server; the homelab agent refused, correctly, and Norbi had to pass the task on by hand.

Norbi's decision (2026-10-26): keep the safety, remove the manual step — **such a task becomes an approval request to the humans; when an admin approves, the full-access agent gets it as if a human had given it.**

Read first: `CLAUDE.md`, `docs/01-terv.md` ("Ki adhat feladatot kinek"), `internal/guard/direct.go` (`humanOnly`, how refused items are acked and noted; the answer-item exception), `internal/modules/delivery/` (`onMessage`, flags, item kinds, `threadFollowers`), `internal/modules/question/` (Ask/Answer, `ToHuman`, options, events `question.answered`/`expired`, the inline answer UI on the dev page, the "Válaszra vár" tab), `internal/modules/hosts/` (an agent's assignment: `sandbox`), `internal/modules/conversation/` (`MessageCreated`, `NoThreadFollow`), `internal/httpserver/web/index.html`.

Do the parts in this order; one commit (or a few) per part; after each part `gofmt`, `make test`, `make lint`, `make test-web` if the page changed. `make test-pg` uncached (`GOFLAGS=-count=1`; if the server image does not build here, start only the Postgres container) for the new store; `go test -race ./internal/guard/ ./internal/e2e/`. Contract first. Migrations: the next free numbers. **Commit only when `make test` and `make lint` pass.**

## Decisions (Norbi, 2026-10-26; do not change them)

- **When:** an agent's message would reach a **full-access** agent as a task — a mention of it, a DM to it, or (round 2026-10-26) a reply in a thread it follows. Sandboxed targets are unchanged (agents may task them directly). Answers to the target's own questions (`Kind == "answer"`) are unchanged.
- **Instead of waking the target:** the server creates a **delegation request** and asks the humans: a question to humans (`Q-NN`, `to_human`) in the same channel/thread, with two options **"Jóváhagyom" / "Elutasítom"**, saying who asks whom to do what (the task text quoted as data). It appears inline in the chat and in the "Válaszra vár" tab, like any question.
- **Only an admin human's answer decides.** On **approve**: the target gets the original message as a task, marked **"@projects kérése, jóváhagyta: @norbi"**, and its guard runs it like a human's request. On **deny**, or when the question **expires** (24 h): the target is never woken; the source agent gets an answer item ("@norbi elutasította" / "lejárt").
- One request per (message, target). A message that mentions two full-access agents makes two requests.
- The guard keeps refusing any agent-authored task that did not come through an approval (defence in depth).

## 1. Server: the delegation module

- New module `internal/modules/delegation/` (model, store with memory + Postgres + storetest, service, events), reacting to events, not called directly:
  - on `message.created` by an **agent**: for each recipient that is an agent with an assignment `sandbox == false` and would get the message (mention, DM, thread follow — reuse the delivery module's recipient rules through a narrow exported function, do not copy them) → create `delegation{id, workspace_id, message_id, channel_id, thread_root_id, from_agent_id, to_agent_id, question_id, state: pending|approved|denied|expired, decided_by, decided_at, created_at}` and ask the question (through the question module's API, `to_human`, options, deadline 24 h, the text: "@projects feladatot adna @agy-homelab-nak: »…« Jóváhagyod?" with the task quoted and clipped to 500 characters).
  - **Delivery must not wake the full-access target for that message** (no inbox row for it from this message), while every other recipient is unchanged. Decide how the delivery module learns this (e.g. a narrow port the delegation module implements: "does this agent need approval for tasks from agents?"), and explain it in the report.
  - on `question.answered` for a delegation's question: if the answerer is an **admin human** → `approved` with `decided_by`; then create the target's inbox item of a new kind `delegated` (`FlagWake`) for the original message, carrying the approver; otherwise (an answer by a non-admin) → leave it pending and tell the answerer (a reply in the thread: "csak admin hagyhatja jóvá"). "Elutasítom" by an admin → `denied`. On `question.expired` → `expired`. Deny/expire → an answer item to the source agent.
  - State changes only from `pending`, each in one transaction with its event (transactional outbox), conditional updates (as rounds 16/18 do).
- `GET /api/v1/delegations?state=pending` (humans) for the UI; audit every decision.
- Tests: storetest; e2e over the contract — an agent mentions a full-access agent → a pending delegation and a question, the target's inbox is empty; admin approves → the target gets exactly one `delegated` item with the approver; deny → nothing for the target, an answer item for the source; expire (manual clock) → same; a sandboxed target is woken directly as today, no delegation; a human's mention of a full-access agent is unchanged; two full-access agents mentioned → two requests; a non-admin human's answer does not approve; an agent cannot answer the question (existing rule).

## 2. Guard

- `humanOnly` keeps items whose kind is `delegated` (and still requires `AuthorKind == "human"` or `answer` for everything else). The prompt frames it: `<colonyhall_message from="@projects" delegated_by="@norbi">…</colonyhall_message>` plus one line: "This task was written by the agent @projects and approved by @norbi; do it as if @norbi had asked." The text stays data in the frame; the approval is what makes it a task.
- The short note the guard posts today for a refused agent task changes to: "Ezt a feladatot jóváhagyásra küldtem (@norbi)." — but only when the server says a delegation exists for it; otherwise (an old-style item) keep today's note.
- Tests (fake server): a `delegated` item runs as a task with the frame and the approver line; an agent item without approval is still refused; the run's reply goes to the original thread.

## 3. Dev page

- The delegation question renders like other questions, with the two option buttons; after the decision the line shows "✅ Jóváhagyta @norbi 14:05" / "⛔ Elutasította …" / "⌛ Lejárt".
- The target's reply thread is the original message's thread, so the human sees request → approval → work in one place.
- All strings via I18N (hu, en).

## Out of scope (say so in the report)

Standing approvals ("always allow @projects → @agy-homelab"), approval by agents, delegation between humans, editing the task text before approving.

## Docs

Hungarian: `docs/01-terv.md` ("Ki adhat feladatot kinek": the approval path), `docs/02-muszaki-specifikacio.md` (the delegation module, its events, the `delegated` item, the guard rule), `CLAUDE.md` (the "Feladatadás és hibák" paragraph: agents may now request tasks for full-access agents through an admin's approval), `task.md` (move the plan item to "Kész" with the date; keep the commit ids). Then the report `docs/handoff/24-delegation-approval-report.md` (Hungarian): commits, decisions where this prompt was open (how delivery learns about approval), what is left out, the outputs of `make test`, `make lint`, `make test-web` (if run), `make test-pg` (uncached), `make gen-check`, the `-race` run, a manual check on the dev stack (two agents, one full-access; the approval question, approve, the work in the thread), and what the reviewer should look at first.
