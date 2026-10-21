# Handoff prompt 15: sidebar, "working…" indicator, channel sections, channels with members, profiles, agent panel

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/ui-round ../new-chat-worktrees/ui-round main
cd ../new-chat-worktrees/ui-round
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/15-ui-round-prompt.md)"
```

---

Branch: `feature/ui-round`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/ui-round` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/ui-round`; never merge, never push.

Background: `task.md` (repository root) lists what Norbi found while using Colonyhall; this round covers the items below. Read first: `CLAUDE.md`, `task.md`, `docs/01-terv.md` (permissions, "Ki adhat feladatot kinek"), `internal/httpserver/web/index.html` (the dev page: `el()` helper, `showPanel()`, `renderChannels()`, `renderHosts()`, the I18N blocks), `internal/modules/conversation/` (channels, members, policy), `internal/modules/identity/` (actors), `internal/modules/hosts/` (assignments, grants), `internal/modules/runs/` (runs, usage, budget), `internal/guard/direct.go` and `guard.go` (direct and work runs), `internal/policy/policy.go`.

The production server runs on the homelab and the dev stack on this machine is normally stopped. For `make test-pg` start it from the worktree (`make dev`) and **stop it at the end (`make down`)**; it restarts with Docker otherwise.

Migrations: the next free numbers at the time you start (`main` ends at `00021`).

Do the parts in this order; one commit (or a few) per part; after each part `gofmt`, `make test`, `make lint`.

## 1. Sidebar: items one under the other (bug)

In the left sidebar the channels and DMs end up side by side. Cause: `nav div { display:flex; justify-content:space-between; … }` also matches the list container `#channels` (and `.dmbox`), so the container lays its children out in a row. Fix it so that only the list items are flex rows and the container is a column; every channel and DM on its own line, the unread/mention counter on the right of its row, as in any chat app. Check desktop and phone width (≤ 640 px).

## 2. "Working…" indicator

While an agent works nothing is visible. Add a short-lived activity signal:

- **Guard → server:** `POST /api/v1/agent/activity` (agent token) `{"channel_id", "thread_root_id"?, "state": "working" | "idle"}`. The guard sends `working` when a **direct run** or a **work run** starts (not for the tool-less compact decision run), repeats it every 20 s while the run lasts, and sends `idle` when it ends (also on error, halt and timeout). Best effort: a failed call is logged, never blocks or fails the run.
- **Server:** not stored, not in the event log, not audited, wakes nobody. It keeps the current state in memory per (agent, channel, thread) with a 60 s expiry (a guard that dies stops showing as working) and pushes an ephemeral WebSocket frame `agent.activity` `{agent_id, channel_id, thread_root_id, state}` **only to viewers who can read that channel** (check `CanRead` per recipient, like other channel events). The agent must be able to read the channel it reports for, else 403. The clock comes from the `Clock` port.
- **Dev page:** one line above the message box: "@trader-logs dolgozik…" (several agents: "@a, @b dolgozik…"), only for the open channel or thread; it disappears on `idle` or after the expiry. i18n keys hu/en.
- Tests: guard (fake adapter) sends working/idle around direct and work runs, not around the decision run, and keeps going when the call fails; server: expiry, 403 for a channel the agent cannot read, a viewer without access gets no frame; e2e over the contract.

## 3. Sections in the sidebar (like Slack sections / Discord categories)

Decisions (Norbi may still change them; keep it simple): sections are **personal** (each human arranges their own sidebar), a channel or DM is in **at most one** section, items without a section stay in the default "Csatornák" / "Közvetlen üzenetek" groups.

- Table (append-friendly: soft delete with `deleted_at`, no hard delete): `sidebar_section` (`id`, `workspace_id`, `owner_id`, `name` ≤ 40, `position`, `collapsed`, timestamps, `deleted_at`) and `sidebar_item` (`section_id`, `channel_id`, `position`).
- REST for humans only (agents get 403): list my sections with their items; create, rename, reorder, collapse/expand, delete a section (its items fall back to the defaults); put a channel/DM into a section or take it out; reorder items. Only the owner sees and changes their sections; a channel the owner cannot read must not appear.
- Dev page: sections in the sidebar with collapse arrows; "+ szekció"; per item a small menu or drag-free controls ("áthelyezés ide…", "fel/le") — no drag-and-drop needed.
- storetest (memory + Postgres) and e2e (another human's sections are invisible; an agent gets 403).

## 4. Named channels with chosen members

The backend has public/private channels and `POST /channels/{id}/members`; the dev page cannot create a channel with members or manage them.

- Channel gets a `description` (≤ 250, optional): migration, `CreateChannelRequest.description`, returned in `Channel`.
- `GET /api/v1/channels/{id}/members` (anyone who can read the channel): members with handle, kind, `can_write`.
- `DELETE /api/v1/channels/{id}/members/{actorId}` (admins only, like adding): soft-remove (the membership row gets `left_at` or equivalent; history stays); removing the last human admin of a private channel is refused. Event + audit like adding.
- Who may create: humans only (unchanged: agents never create channels).
- Dev page: "+ új csatorna" dialog: name, description, public/private, member picker (checkbox list of agents and humans, filterable); in the channel header a "tagok" button that lists members with add/remove for admins.
- Tests: an agent cannot create a channel or manage members; a non-admin human cannot add/remove; a removed member loses read access to a private channel (CanRead) and stops receiving its inbox items; e2e over the contract.

## 5. Richer profile

- Actor gets optional profile fields: `description` (≤ 300, a role/one-line bio), `avatar_emoji` (one emoji; no image upload in this round), `timezone` (IANA name, validated with `time.LoadLocation`, default `Europe/Budapest`), `status_text` (≤ 80). Migration; `Actor` schema; `PATCH /api/v1/actors/{id}/profile`: a human edits their own profile, admins edit anyone's (agents' too); agents cannot edit profiles. Changes are events + audit.
- The guard does not need these.
- Dev page: the avatar emoji next to names in the feed and sidebar; a "Profil" editor for yourself (header menu or the panel of part 6).
- Tests: field limits, timezone validation, who may edit.

## 6. Agent panel on the right (like Slack's profile panel)

Clicking an agent's (or human's) name in the feed, the sidebar or the member list opens a wider panel on the right side; on phones it is a full-screen sheet with a close button. Read-only by default; actions only for admins.

- New aggregate endpoint `GET /api/v1/agents/{id}/overview` (humans only; agents 403), one call for the panel, built from the existing modules through their root APIs:
  - actor and profile (part 5), state (active / paused / stopped);
  - assignment: machine, client, model, mode (direct / compact), sandbox, root folder, folder grants (path, read/write), worktree repo and branch (from the host report), permission mode, approvals, `max_session_tokens`, `daily_token_limit`;
  - context per thread: the latest run per thread (thread link, `session_id`, tokens of the last run vs `max_session_tokens`, so the panel can show "112k / 150k — new session soon");
  - usage: today's tokens vs the daily limit, the last 10 runs (time, phase, result, tokens, error id link for humans);
  - collaboration: the agent's notes (latest per thread), active leases, open questions it asked or must answer;
  - activity: whether it is working right now (part 2).
- Admin actions in the panel: pause / resume / stop (existing endpoints), "beállítások" (opens the existing host assignment form), folder grants add/remove (existing endpoints).
- A human's panel: profile, role, their channels in common.
- Tests: an agent gets 403; another workspace's agent is 404; the response is checked against the contract; numbers match the stores.

## 7. Small agent-prompt fixes (from `task.md`, "Nyitott")

- **Sandboxed agents name their real folders:** the work-run prompt (`WorkPrompt`) lists the granted folders with read/write, e.g. "You can read /home/…/trader (read-only) and /…/trader-data (read-only); nothing else is visible." Today they report their sandbox directory as their working folder.
- **"Longer, please" is taken seriously:** the direct prompt's length line becomes: "Keep the answer short (≤600 characters) unless someone asks for more; when asked for a longer summary or report, you may write up to 8000 characters."
- **Same language:** the work and direct prompts say: "Answer in the language of the message you are answering; do not mix languages."
- Guard tests for the three prompt texts.

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (new endpoints, the WebSocket `agent.activity` frame, sections, channel description and member removal, profile fields, the overview), `docs/01-terv.md` (mark what is implemented), `task.md` (move the done items to "Kész" with the date; keep the commit ids for the reviewer to fill in), `CLAUDE.md` schedule. Then the report `docs/handoff/15-ui-round-report.md` (Hungarian): commits, decisions where this prompt was open, what is left out, the results of `make test`, `make lint`, `make test-pg`, `make gen-check`, the `-race` run, screenshots or a description of the dev page changes at desktop and phone width, and what the reviewer should look at first.
