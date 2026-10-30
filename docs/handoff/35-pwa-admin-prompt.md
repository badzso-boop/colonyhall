# Handoff prompt 35: PWA admin — runs and usage with estimated cost, settings, users, tokens and the owner key, audit, system status, emergency stop and channel locking

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/pwa-admin ../new-chat-worktrees/pwa-admin main
cd ../new-chat-worktrees/pwa-admin
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/35-pwa-admin-prompt.md)"
```

---

Branch: `feature/pwa-admin`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/pwa-admin`. Never work in `/mnt/g/Projects/new-chat` itself, never touch `main`; commit only on this branch, never merge, never push. **Rounds 31–34 must be merged first.** Read `docs/07-design-system.md` and follow it, including its dataviz rules for every chart. This round **needs migrations** (next free numbers) and **changes the guard** (part 7, channel locking): deploy the server first, then the guards.

After each part: `gofmt`, `make test`, `make lint`, `make test-web`, `make gen-check` if the contract or SQL changed. Commit only when they pass. At the end `make test-pg` uncached and `go test -race ./internal/guard/ ./internal/e2e/`. Every authorization rule needs a test that fails without it. Check every screen in a headless browser at 1440×900, 1024×768 and 390×844 in both themes; save the screenshots under `docs/handoff/35-screens/`.

## Why, and Norbi's decisions (2026-10-30)

This is the last round before the dev page can be retired: everything an admin does on it moves to the PWA, plus a few things it never had.

Decisions, binding for this round:

1. **Emergency-stop release requires the password again.** This is already enforced by the server (`/admin/emergency-clear`); the UI must ask for it. Passkeys come later and will plug in at the same place.
2. **Channel locking is an agent lock:**
   - in a locked channel, agents cannot write and are not woken, and their running work there stops;
   - humans keep talking;
   - one click unlocks it.
3. **Usage shows tokens and an estimated cost in USD and HUF,** from an admin-maintained price table and a configurable exchange rate.
4. **For subscription runs** (Claude Code, Antigravity), the cost means **"what this would cost on the API"**, clearly labelled as an estimate under a subscription, next to the Claude 5-hour and weekly limit usage. API-key agents show their estimated actual cost.
5. **Also in this round:** user management, a system status page, audit export (CSV), and a guided owner key rotation.
6. **Only the owner** creates and changes human users and roles. An admin cannot make another admin.

## 1. Navigation: the admin area

The sidebar's "Munkaterület" list gets an **"Adminisztráció"** group for admins:

- Futások;
- Beállítások;
- Felhasználók (owner only);
- Tokenek;
- Audit;
- Rendszerállapot.

Each is a page under `/app/admin/…`. Machines and agents stay where round 33 put them. Non-admins get 404-style EmptyStates from the client and 403 from the server.

## 2. Runs and usage: `/app/admin/runs`

### 2.1 Server (contract first)

- **Price table** (migration):
  - `model_price` with `model` (an exact name or a prefix pattern ending in `*`), `input_per_mtok`, `output_per_mtok`, `cache_read_per_mtok` and `cache_write_per_mtok` (USD, numeric), `valid_from`, `created_by` and `created_at`.
  - **Append-only:** a price change is a new row with a new `valid_from`, so old runs keep the price of their time. Matching picks the longest pattern valid at the run's time.
  - Endpoints: `GET /settings/prices`, `POST /settings/prices` (admins; audited).
  - **No invented defaults:** seed nothing. The UI shows "nincs ár" until Norbi fills it. You may offer a "Javasolt értékek" button only if the values come with a source URL shown next to them; otherwise leave it out.
- **Exchange rate:** `PUT /settings/currency` with `usd_huf` (positive number, `updated_at`; admins; audited). It is a manual setting: no external rate service, the server is Tailscale-only.
- **Estimated cost on runs and usage** (additive fields):
  - `Run.estimated_usd` (nullable when no price matches) and `Run.billing` (`subscription` for `claude-code` and `antigravity`, `api` for API-key agents);
  - `UsageResponse` gets the per-agent-and-day `estimated_usd` and the totals.
  - **HUF is computed in the client** from `usd_huf`, so changing the rate never rewrites history.
  - The cost is computed in the server from the four token counts with the price valid at `created_at`, in one pure function with table tests: no price, a prefix match vs. an exact match, a price change between two runs, cache tokens.
- **Run filters:** `GET /runs` gets the optional query parameters `agent_id`, `host_id`, `status`, `from`, `to`, `channel_id`, and a cursor (additive). `GET /usage` allows `days` up to 90 (already) and gets `group_by=agent|host|model`.

### 2.2 The page

- **Header:**
  - the period (Ma / 7 nap / 30 nap / 90 nap / egyéni);
  - **KPI tiles:** tokens, runs, failures, average duration, **estimated cost** ("API-n ~$12.40 · ~4 520 Ft", with the "Előfizetésben fut" note when relevant);
  - the Claude 5-hour and weekly limit rings per machine (round 27).
- **Charts:**
  - tokens per day, stacked by agent (or by machine or model; a switch), with the agent colours from round 33;
  - a runs-per-day bar chart with failures highlighted.
  - Follow the dataviz rules of the design system, with tooltips and both themes.
- **Table:** agents with runs, tokens (in, out, cache), estimated cost, failures, last run, and a sparkline. Sorting is on every column.
- **Run list:**
  - the filters from 2.1, with infinite scroll;
  - each row: state icon, agent, phase, thread link, start, duration, tokens, estimated cost, `🌐 N`;
  - opening a row shows a Sheet with the step list, the session, the context size, the instructions version (round 33) and, for failed runs, the error details (`/errors/{id}`, humans only).
- **Budgets:** each agent's `daily_token_limit` as a bar against today's use. Editing goes to the Keretek tab of the agent settings Dialog (round 33).

## 3. Settings: `/app/admin/settings`

One page with a left section list (tabs on a phone). Each section saves on its own with a "Mi változik" summary.

- **Tömörítés** (`/settings/compaction`): the workspace threshold (350k default) with presets, and the scribe agent.
- **Fájllekérés** (`/settings/file-fetch`): the round 16 limits.
- **Feladatátadás** (`/delegation-settings`, round 25): the quiet timeout (5–1440 min) and the maximum length (1–72 h) with sliders and numeric fields; below them, **the active grants** (`/delegation-grants?active=true`) with revoke.
- **Gitea** (`/git/mappings`): the repo → channel mappings as a table, add (repo field with `owner/repo` validation, channel picker) and remove. A read-only status line says whether `COLONY_GITEA_URL` is set and whether the webhook secret is set (yes/no only, never the value).
- **Árak és pénznem:** the price table (2.1) as an editable list. Saving adds new rows; old rows stay visible as history. The USD→HUF rate is a field with its last change shown.
- **Fájlok:** the read-only `COLONY_MAX_UPLOAD_MB` and the storage directory size if the server already knows it; otherwise leave the size out.

## 4. Users: `/app/admin/users` (owner only)

- **Server** (contract first, where missing):
  - creating a human user (handle, display name, role `admin` / `member` / `observer`, an initial password the owner types or a generated one shown once);
  - changing a role;
  - disabling a user (`stopped` via `/actors/{id}/state`, or a dedicated `disabled` if `stopped` does not fit humans: say which in the report);
  - resetting a password (the existing `/actors/{actorId}/password` admin path, restricted here to the owner for other humans).
  - **Only the owner may call these for humans.** An admin gets 403, including for making someone admin. The owner cannot demote or disable themself. Every change is audited. Write tests for each rule.
- **Page:**
  - a table of humans: avatar, name, `@handle`, role chip, state, last seen, number of active sessions;
  - actions: "Szerepkör", "Jelszó visszaállítása", "Letiltás" / "Engedélyezés", each in a Dialog;
  - "Új felhasználó" is a Dialog; a generated password is shown once with copy and a warning.
- **Observer:** state in the UI what an observer can do (read only) next to the role select.

## 5. Tokens and the owner key: `/app/admin/tokens`

Over `/actors/{actorId}/tokens` and `/tokens/{tokenId}`:

- **Mine:** your own tokens (name, created, last used if known, prefix only), "Új token" (a name; the secret is shown once with copy and a warning) and revoke (confirmation Dialog).
- **Others** (admins, for actors of lower rank as the server allows): a filter by actor, revoke.
- **Guided owner key rotation** (`task.md`: the owner API key was in a conversation and should be replaced). A three-step wizard for the owner:
  1. "Új kulcs létrehozása" (shown once, copy, "Elmentettem" checkbox to continue);
  2. "Hol használtad a régit?", a checklist the owner ticks (scripts, `.env` files, other machines), with the hint that secrets go through `colonyhall-exchange` and never through the chat;
  3. "Régi kulcs visszavonása", which shows the old token by name and prefix and revokes it after a confirmation.
  - The wizard can be left and resumed (the state is per viewer, `localStorage`; the secret itself is never stored).
  - After it finishes, mark the `task.md` item done in the report, not in code.

## 6. Audit: `/app/admin/audit`

Over `/admin/audit` and `/admin/audit/verify`:

- **The table:** time, actor (avatar), action, target (resolved to a name and a link where possible), and an expandable row with the payload as a key/value list. Never render the payload as HTML.
- **Filters:** actor, action (multi-select from the known actions), target type, and a time range. Add these as optional query parameters to `/admin/audit` (additive; contract first; store query; tests). Paging with the existing `before` cursor.
- **"Lánc ellenőrzése"** runs verify and shows ✅ or the first altered entry, highlighted.
- **Export:** `GET /admin/audit/export?…` with the same filters returns **CSV** (UTF-8 with BOM for Excel; columns seq, at, actor handle, action, target type, target id, payload as compact JSON). It streams, has a 100 000 row limit (with a header comment when truncated), and is admins only and audited ("audit exported"). The page has a "CSV letöltése" button that uses the current filters. e2e tests: the content type, the header row, the filter applied, and 403 for non-admins.

## 7. Emergency stop and channel locking

### Emergency stop

- The header's HoldButton (round 31) calls `/admin/emergency-stop`. While it is active, the header turns red (round 31).
- **"Feloldás"** opens a Dialog that **asks for the password**: an error stays in the dialog, and lockout or rate limit messages come from the server. It calls `/admin/emergency-clear`. Leave a clearly marked place for the later passkey option.
- The status page (part 8) shows who stopped it and when.

### Channel locking (agent lock)

- **Server** (contract first; migration `channel.agent_locked_at`, `agent_locked_by`):
  - `POST /channels/{channelId}/lock` and `/unlock` (admins); events `channel.locked` and `channel.unlocked`; audit;
  - a silent system message ("🔒 @norbi zárolta a csatornát az agentek elől" / "🔓 feloldotta").
- **While locked:**
  - agent writes to the channel (REST and MCP: send, react, upload, `wiki`/`decision` writes bound to it) are refused with `channel_locked`;
  - **no wake items** are created for agents in it;
  - pending wake items are kept but do not wake until unlocked (decide whether to keep or ack them, prefer keeping, and test it);
  - reads stay allowed.
  - Humans are not affected.
  - Schedules posting as a human still post.
- **Guard:** on `channel.locked`, stop the running work in that channel, as round 34 does for removal and archive, and reuse that code.
- **UI:**
  - "Zárolás az agentek elől" in the channel menu and the channel settings Dialog (round 34);
  - a locked channel shows a 🔒 in the sidebar and an amber bar under the header ("Az agentek nem írhatnak és nem ébrednek ebben a csatornában", with "Feloldás");
  - agent avatars in it are dimmed.
- Tests:
  - agent send, react and upload refused;
  - human send allowed;
  - no wake item while locked;
  - the wake after unlock follows the decision above;
  - the guard stops a running run;
  - 403 for non-admins.

## 8. System status: `/app/admin/status`

- **Server:** extend `GET /admin/status` (additive):
  - server version and uptime;
  - database reachable and its latency;
  - the event log head and each consumer's offset and lag (`consumer_offset`);
  - the number of open WebSocket connections by kind (human, agent, host);
  - the guards (hosts with `last_seen_at`, guard version, online flag);
  - the scheduler (next schedule run, failed schedules in the last 24 h);
  - compaction tasks running or failed;
  - the emergency stop state;
  - the file storage (directory size if cheap; otherwise omit).
  - Admins only. **No secrets, no environment values**, only "beállítva: igen/nem" for optional integrations (Gitea).
  - Backups: the server does not know about them today. Show "a mentéseket a homelab kezeli" with a link to `docs/05-homelab-telepites.md` text, and do not invent a check.
- **Page:** a grid of status cards (ok / warning / error with icon and text, never colour alone):
  - consumer lag warns above 100 events or 60 s;
  - a guard is offline after 90 s;
  - a failed schedule is a warning.
  - It auto-refreshes every 10 s while visible. "Másolás" copies a plain-text status summary for a bug report.

## Not in this round

Push notifications, notification settings and quiet hours; passkeys; message search; retiring the dev page (a separate, small round after Norbi has used the PWA for a while); external exchange rate services; backup monitoring.

## Docs and report

- Update `docs/01-terv.md` (channel locking as decided, cost estimation, user management rules), `docs/02-muszaki-specifikacio.md` (all endpoints, tables, events, the CSV export, the status fields), `docs/06-allapot-es-uzemeltetes.md` (the status page as the first place to look when troubleshooting), `docs/07-design-system.md` (the admin page layout, KPI tiles, status cards, wizard), and `task.md`.
- In the report, as usual, and also:
  - the guard deploy order;
  - a short checklist of what the PWA now covers vs. the dev page, so Norbi can decide when to retire it;
  - screenshots of:
    - the runs page with charts and a run Sheet;
    - the price table;
    - each settings section;
    - the users table and the new-user Dialog;
    - the owner key wizard steps;
    - the audit table with a filter and the verify result;
    - the emergency stop release Dialog;
    - a locked channel;
    - the status page with a warning.
