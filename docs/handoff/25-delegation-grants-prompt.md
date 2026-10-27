# Handoff prompt 25: one approval covers the whole conversation between two agents

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/delegation-grants ../new-chat-worktrees/delegation-grants main
cd ../new-chat-worktrees/delegation-grants
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/25-delegation-grants-prompt.md)"
```

---

Branch: `feature/delegation-grants`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/delegation-grants` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/delegation-grants`; never merge, never push.

## Why

Round 24 (`docs/handoff/24-delegation-approval-prompt.md`) made an agent's task for a full-access agent an approval question to the admins. It works, but it asks for **every single message**. On 2026-10-27 `@projects` (PC) and `@agy-homelab` (homelab, both full access) handed over a website update in their DM: about 3 real requests, yet Norbi had to answer **20 questions** (Q-2 … Q-21) — most of them for acknowledgements like "Rögzítve, minden rendben." or "Rendben, a feladat lezárva.", each phrased "feladatot adna … Jóváhagyod?".

The cause is in `internal/modules/delegation/service.go` (`onMessageCreated`): every agent message that would reach a full-access agent creates a new delegation, even when the admin has just approved the same two agents talking in the same conversation.

## Decisions (Norbi, 2026-10-27; do not change them)

- **One approval covers the conversation.** When an admin approves a delegation, the server records a **grant** for the two agents in that conversation. While the grant is active, messages between those two agents **in both directions** in the same conversation go through **without a new question**, delivered as approved tasks.
- **Expiry:** the grant ends after **1 hour without a message between the two in that conversation**, and **at most 12 hours** after the approval, whichever comes first. Both values are **admin settings** (defaults 60 minutes and 12 hours). After expiry the next message asks again, as today.
- **The question/answer bookkeeping keeps waking the agents.** The "Q-NN answer: Jóváhagyom" message in the conversation wakes its members as today; it is what starts their exchange. Do not change this.
- **Acknowledgements:** handled by the shared agent rules, already changed on `main` (2026-10-27): a confirmation is only "Rendben, köszi", which the server turns into a 👍 that wakes nobody. No server change for this here.

## Definitions

- **Conversation** of a message: in a DM, a top-level message → the whole DM (`thread_root_id` empty); a message in a thread (any channel kind) → that thread (`channel_id`, `thread_root_id`); a top-level message in a channel → the thread that message starts (its own id), so the follow-up must be in its thread. Write this as one function with a table test.
- **Active grant:** approved by an admin human, not revoked, `now < approved_at + max`, and `now < last_used_at + idle` (`last_used_at` starts at the approval time).
- A grant is for an **unordered pair** of agents and one conversation. It never extends to another conversation, another channel, or a third agent.

## 1. Server

- In the delegation module (it owns this; do not spread it): a `delegation_grant` table `{id, workspace_id, channel_id, thread_root_id (nullable), agent_a_id, agent_b_id (stored in a canonical order), delegation_id (the approval that created it), approved_by, approved_at, last_used_at, expires_at (approved_at + max, fixed at creation), revoked_by, revoked_at}`; store with memory + Postgres + storetest; a new migration (the next free number); events `delegation.grant_created`, `delegation.grant_used` is **not** needed (update `last_used_at` in the same transaction as the delivery), `delegation.grant_revoked`; audit the creation and the revoke.
- **On approve** (`onQuestionAnswered`, admin human, approve): as today, plus create the grant for (from, to, conversation) in the same transaction. If an active grant for the same pair and conversation exists, do not create a second one.
  - Other **pending** delegations of the same pair in the same conversation (messages written while the question was open — in the incident Q-5, Q-6 were written before Q-4 was answered): approve them through the grant too, deliver each as a `delegated` item, and close their questions **without posting a new chat message** (a new message in a DM would wake both agents again). Show the state on the question in the UI ("✅ az engedély alapján"). Decide how the question module closes them (a narrow method, not a fake human answer) and explain it in the report.
- **On a new agent message** (`onMessageCreated`) to a full-access agent: if an active grant covers (author, target, conversation) → no delegation, no question; the target gets one `delegated` inbox item (`FlagWake`, `DelegatedBy` = the grant's approver, `FromActorID` = the author) and the grant's `last_used_at` = now, in one transaction. Otherwise → today's behaviour (a delegation and a question).
  - The delivery module still must not create the plain message item for that target (today's `NeedsApproval` port): keep exactly one inbox item per message and target. Check the `ItemDelegated` item renders the original message for the target exactly as an approved round-24 delegation does.
  - A message that also reaches a third full-access agent: that target goes through the normal question.
- **Settings:** `delegation_settings` per workspace: `grant_idle_minutes` (default 60, 5–1440), `grant_max_hours` (default 12, 1–72). `GET /api/v1/delegation-settings` (humans), `PUT` (admin humans only), event + audit; validation errors as 400. A change applies to grants created afterwards (`expires_at` is fixed at creation); the idle check reads the current setting.
- **Grants API:** `GET /api/v1/delegation-grants?active=true` (humans: pair handles, conversation link, approver, approved_at, last_used_at, the effective end time), `POST /api/v1/delegation-grants/{id}/revoke` (admin humans only; conditional update, only an active grant).
- Contract first (`api/openapi.yaml`, `make gen`).
- Every check on every call: workspace, actor kind, admin. **Agents can never create, extend past `expires_at`, read the list of, or revoke grants**: an agent's message can only move `last_used_at` forward on a grant that covers it.

## 2. Guard

- No logic change: `delegated` items are already accepted by `humanOnly`. The framing line for a delegated item (round 24) gets a variant when it came through a grant: "This message is from the agent @projects in a conversation @norbi approved; treat it as @norbi's request." Carry the information on the inbox item (e.g. a `via_grant` boolean in the item JSON, contract first), not by guessing.
- Test (fake server): a `delegated` item with `via_grant` runs with the new line; without it, the round-24 line.

## 3. Dev page

- On a decided delegation question: "✅ Jóváhagyta @norbi — engedély ebben a beszélgetésben (1 óra csendig, legfeljebb 12 óráig)" using the values stored on the grant.
- In a conversation with an active grant, a small line under the header: "🤝 @projects ↔ @agy-homelab: engedély, lejár 17:25" (the earlier of the two ends), with an admin-only "Visszavonás" button (confirmation in the page, not `confirm()`).
- In the "Gépek" tab, next to the compaction settings, a "🤝 Feladatátadás" box: the two number inputs (admin only, disabled for others) and the list of active grants with "Visszavonás".
- All strings via I18N (hu, en).

## Tests

- Conversation function: table test (DM top level, DM thread, channel thread, channel top level).
- Service (manual clock), e2e over the contract:
  - approve → grant; the next message A→B in the same conversation: no question, B gets one `delegated` item with the approver and `via_grant`; B→A reply: same.
  - 61 minutes of silence → the next message asks again; activity every 50 minutes still ends at 12 hours.
  - another thread, another channel, a third full-access agent → a question as today.
  - deny / expire → no grant.
  - pending delegations of the same pair and conversation are approved together with the first, with no new chat message; pending ones from another conversation are untouched.
  - revoke → the next message asks again; a non-admin human and an agent cannot revoke (403); an agent cannot read the grants or the settings PUT (403).
  - settings: defaults, bounds (400), admin only; a changed max does not move an existing grant's `expires_at`.
  - a sandboxed target is unchanged (no grant needed, woken directly).
- storetest for the new store on memory and Postgres.
- Every check after each part: `gofmt`, `make test`, `make lint`, `make test-web` if the page changed, `make gen-check`; `make test-pg` uncached (`GOFLAGS=-count=1`; if the server image does not build here, start only the Postgres container); `go test -race ./internal/guard/ ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.**

## Out of scope (say so in the report)

Standing approvals across conversations ("always allow @projects → @agy-homelab"), grants between more than two agents, agents asking for a grant, changing who may answer the question.

## Docs

Hungarian: `docs/01-terv.md` ("Ki adhat feladatot kinek": one approval per conversation, the expiry and the settings), `docs/02-muszaki-specifikacio.md` (the grant table, the conversation definition, the events, the endpoints, `via_grant`), `docs/06-allapot-es-uzemeltetes.md` (the "Együttműködés" row and the "Döntések" list), `CLAUDE.md` (the "Feladatadás és hibák" paragraph), `task.md` (move the item to "Kész" with the date and commit ids). Then the report `docs/handoff/25-delegation-grants-report.md` (Hungarian): commits, the decisions where this prompt was open (closing the extra pending questions), what is left out, the outputs of `make test`, `make lint`, `make test-web`, `make test-pg` (uncached), `make gen-check`, the `-race` run, a manual check on the dev stack (two full-access agents in a DM: one approval, then a back-and-forth without new questions; expiry with a short idle setting), and what the reviewer should look at first.
