# Handoff prompt 09: questions and answers (ask / answer / escalation)

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/09-ask-answer-prompt.md)"
```

---

Branch: `feature/ask-answer`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 3, and the section "Kérdés–válasz protokoll és „kérdezz embert”" (older, broader; item 3 wins where they differ; no task objects yet). Read `internal/modules/schedule/` (a recent small module with a background loop), `internal/modules/conversation/mcptools/`, `internal/modules/delivery/` (inbox items and kinds), `internal/guard/direct.go` (`humanOnly`).

## Server (new module `question`)

- Table `question` (append-only except the conditional answer/escalation updates): `id`, `number` (per workspace, shown as `Q-12`), `workspace_id`, `asked_by`, `to_actor_id` (nullable) or `to_human` bool, `channel_id`, `thread_root_id`, `text`, `options` (text[]), `default_option`, `deadline` (nullable), `state` (`open` | `answered` | `escalated` | `withdrawn` | `expired`), `answer`, `answered_by`, timestamps.
- MCP tools (agents): `ask` (question, to: `@handle` or `human`, options?, default?, deadline_minutes?) → posts the question in the current thread (`Q-12 …`, mentioning the target so it is delivered), returns `Q-12`; `answer` (`Q-12`, text or option) → stores it, posts it in the thread, and puts an inbox item of kind `answer` for the asker; `withdraw` (`Q-12`, only the asker). Humans answer from the dev page (or by replying with `/answer Q-12 …`).
- Rules (test all): an agent asking a **full-access agent** (an agent assigned in direct mode without sandbox) is redirected to humans (`to_human`) — the guard would refuse the task anyway; one open question per thread per asker (a new one needs the old withdrawn); only the target (or any human of the workspace for `human`, or for escalated questions) can answer; a question to an agent unanswered for 30 min (configurable per workspace later; constant now) is **escalated**: state `escalated`, a note in the thread mentioning the workspace admins; a question with `default` and `deadline` to an **agent** falls back to the default at the deadline (the asker is told); a question to a **human** never resolves by itself.
- Background loop like the scheduler's (claim with conditional updates, events, audit).
- REST for humans: `GET /api/v1/questions?state=open` ("Válaszra vár"), `POST /api/v1/questions/{id}/answer`. Contract-first, storetest, e2e.

## Guard

- An inbox item of kind `question` or `answer` is handled like a message; for direct agents `humanOnly` already blocks agent-originated items — keep it so for `question` (redirected on the server anyway) but **allow `answer` items** that answer the agent's own question (they carry the answer, not a task).
- Add one sentence to the direct prompt: when you need a decision, use `ask` (to `human` for anything irreversible) instead of guessing.

## Dev page

"Válaszra vár" in the ⏰ panel or its own button: open questions to humans and escalated ones, with option buttons / a text answer.

## Docs

`docs/02-muszaki-specifikacio.md` (MCP tool list: mark `ask`/`answer` implemented; new paragraph), plan section marked, `CLAUDE.md` schedule. Hungarian.
