# Handoff prompt 33: PWA agents and machines — overview pane, agent directory, immutable agents with a detailed creation dialog, host pages, colours, folder picker

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/pwa-agents-hosts ../new-chat-worktrees/pwa-agents-hosts main
cd ../new-chat-worktrees/pwa-agents-hosts
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/33-pwa-agents-hosts-prompt.md)"
```

---

Branch: `feature/pwa-agents-hosts`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/pwa-agents-hosts`. Never work in `/mnt/g/Projects/new-chat` itself, never touch `main`; commit only on this branch, never merge, never push. **Rounds 31 and 32 must be merged first.** Read `docs/07-design-system.md` and follow it; the design system wins over this prompt, and you note any conflict in the report. This round **needs migrations**: take the next free numbers (`ls migrations`). **This round changes the guard too** (part 6): deploy order is server first, then guards (`CLAUDE.md`).

After each part: `gofmt`, `make test`, `make lint`, `make test-web`, `make gen-check` if the contract or SQL changed. Commit only when they pass. At the end `make test-pg` uncached (`GOFLAGS=-count=1`) and `go test -race ./internal/guard/ ./internal/e2e/`. Every authorization rule needs a test that fails without it. Check every screen in a headless browser at 1440×900, 1024×768 and 390×844 in both themes; save the screenshots under `docs/handoff/33-screens/`.

## Why (Norbi, 2026-10-30)

There are 14 agents on two machines, and the dev page's "Gépek" panel is one long form where everything is typed: paths, models, flags. Norbi wants:

- a worked-out **agent overview pane**;
- **machines** designed to be intuitive, with **more detailed and better separated settings**;
- **colours for machines**, so agents can be told apart by them, and colours for agents too;
- **search among the agents**;
- **an agent's own extra system prompt**, given when it is created;
- **folders are never typed:** the guard reports the folders it allows, they appear in a file-manager-like picker, and Norbi chooses from them.

**Decision (Norbi, 2026-10-30): agents are immutable, as the plan says.** `docs/01-terv.md` "Agent életciklus" and `CLAUDE.md` rule 8: after creation, the agent's configuration does not change.

- **Configuration** includes the role and the system prompt (its own instructions), the client, the model, the host, and the folder and repo access.
- A change means **deleting the agent and creating a new one**.
- Therefore the **creation dialog is detailed** and **warns clearly** that none of this can be changed later.
- A version picker (the plan's agent versions) may come later. This round prepares for it (part 2.3) but does not build it.

Today's server breaks this rule: `PUT /agents/{agentId}/host` rewrites the model, host and permissions in place, and grants can be added and removed at any time. **This round closes that** (part 2).

## 1. Colours for machines and agents

- **A fixed palette, not free hex,** so every colour works in both themes and keeps the contrast rules of `docs/07-design-system.md`. Ten named hues: `red`, `orange`, `amber`, `lime`, `green`, `teal`, `cyan`, `blue`, `violet`, `pink`. Each has tokens per theme: `-solid` (stripes, dots, rings), `-soft` (backgrounds) and `-fg` (text on `-soft`, AA). Add the palette to `tokens.css` and to the style guide, and extend the contrast test to it.
  - The brand red `accent` is not in the palette. `red` is a distinct, slightly cooler red, so a red machine never looks like a mention or a danger.
- **Machine colour:** every host has one. Hosts that exist get a default by creation order. Admins change it on the host page.
- **Agent colour:** optional. Without it, **an agent uses its host's colour**. Admins can set an own colour on the agent (e.g. two agents on the same machine).
- **Where colour shows:**
  - the agent's square avatar gets a 2 px ring in its colour;
  - host chips get a dot;
  - the agent directory and the host list get a 3 px left stripe;
  - mention chips get a tinted background (`-soft`);
  - the sidebar's AI groups show the ring.
  - **Colour is never the only signal:** the host name is always in a tooltip or next to it.
- **Server** (contract first; migration):
  - `host.color` (text, NOT NULL, CHECK in the palette, default by creation order in the migration);
  - `actor.color` (nullable, CHECK in the palette, agents only).
  - Both are **operational state** like the display name and avatar (`docs/01-terv.md` "Agent életciklus": "megjelenített név és avatar"), so they **may** change after creation. Each change is audited and emits an event so open clients repaint.
  - Endpoints: `PATCH /hosts/{hostId}` (`name`, `color`; admins) and `color` in `UpdateActorProfileRequest` (admins only for agents; humans may not set a colour this round).
  - `Host` gets `color`. `Actor` gets `color` and an `effective_color` computed by the server (own colour, else the host's, else null).
  - storetest (memory and Postgres), e2e, and a 403 test for non-admins and agents.

## 2. Immutable agents: one atomic creation, no edits, delete instead

### 2.1 What may change and what may not

| Never after creation (configuration) | May change (operational state, audited) |
| --- | --- |
| host, client, model, work model, mode, `allow_work`, permission mode, sandbox or full access, root folder, worktree repo, **folder grants**, approvals, preview, web_read, **own instructions** | display name, avatar emoji, colour, description, status text, time zone; **limits** (`daily_token_limit`, `max_session_tokens`); paused / running / stopped; channel memberships; **deletion** |

The limits are the plan's "keretek", which it lists as changeable. Note this reading in `docs/01-terv.md`.

### 2.2 Server (contract first)

- **`POST /agents`** (admins) creates a configured agent **in one transaction**:
  - the actor (handle, display name, avatar emoji, colour, description);
  - the assignment (every field of today's `AssignRequest`);
  - the channel memberships (step 5 of the creation dialog);
  - the folder grants;
  - the own instructions;
  - and the audit entries.
  - Validation happens before anything is written: the handle (2–32 characters, lowercase, unique among non-archived actors), the host (approved), the client (one the host reported), the paths (inside the host's reported roots, part 6), and the instructions (part 2.3).
  - It is all or nothing, so a half-made agent cannot exist.
  - The response is the new agent with its assignment. For an "API-kulcsos agent" (no host), it creates only the actor and a token shown once.
- **Closing the old ways:**
  - `PUT /agents/{agentId}/host`, `DELETE /agents/{agentId}/host`, `POST /agents/{agentId}/grants` and `DELETE /grants/{grantId}` now answer **409 `agent_config_immutable`**, for every agent, with a problem detail pointing to `POST /agents` and `DELETE /agents/{agentId}`.
  - Keep the routes and their contract entries (marked deprecated), so old clients get a clear error instead of 404.
  - `createActor` with `kind: agent` is refused the same way; humans are still created there (round 35 moves that to the user page).
- **`PATCH /agents/{agentId}/limits`** (admins): `daily_token_limit`, `max_session_tokens`. Audited, event, e2e.
- **`DELETE /agents/{agentId}`** (admins) **archives** the agent. The plan says a stopped agent is not deleted, only archived, and its messages stay. In one transaction:
  - state `stopped`;
  - `actor.archived_at` and `archived_by` set (migration);
  - every token of the agent revoked;
  - the assignment ended (grants end, the guard drops the agent on its next config fetch);
  - the agent's channel memberships removed silently (no wake, no chat noise);
  - the agent's active delegation grants revoked;
  - its pending delivery items acked without waking.
  - Event `agent.archived`; audit.
  - Archived agents:
    - are left out of `/actors` unless `?archived=true`, out of mention resolution, and out of the sidebar;
    - their old messages keep showing their author, with an "archivált" badge.
- **The handle becomes free again:** replace `UNIQUE (workspace_id, handle)` with a **unique index on (workspace_id, handle) where `archived_at IS NULL`**. Check every query that relies on the old constraint (e.g. `ON CONFLICT`). A new agent can therefore take the old agent's handle, so `@backend-claude` points to the new one. This is the first step towards the plan's agent families: old messages keep the old agent's id.
- **The guard:** when an agent disappears from `GET /host/config` (archived), the guard stops its running decide or work run and **kills its terminals** (round 23's halt semantics, `killed_by: "archived"`), if it does not already. Test it with fake runners.
- **Migrating today's data:** nothing is rewritten. The 14 existing agents keep their assignments; they simply cannot be edited any more.
- **Tests:**
  - **Test helpers:** move `createFullAccessAgent`, `createSandboxedAgent` and every other helper or test that calls the old endpoints to `POST /agents`. List in the report how many tests changed. Behaviour stays otherwise the same.
  - **New tests:**
    - atomic creation (a bad grant path writes nothing);
    - the 409s on every closed endpoint;
    - limits editable;
    - archive (tokens revoked, memberships removed, the guard drops the agent, terminals killed);
    - handle reuse after archive and refusal while the old one is active;
    - mention resolution to the active agent;
    - agents and non-admins get 403 everywhere.
- **The dev page:** its assignment form and grant buttons call the closed endpoints. Make that panel **read-only**, with a hint "Az agentek a PWA-ban hozhatók létre (/app/agents)", and keep its other functions. Update `internal/httpserver/web/*.test.mjs` if they check those forms.

### 2.3 Own instructions (an extra system prompt, set at creation)

- **Storage:** `agent_instructions` (`agent_id`, `version` (always 1 this round), `text` max 8 000, `created_by`, `created_at`; unique (`agent_id`, `version`)). It is written only by `POST /agents`. The `version` column is there for the later version picker; there is no endpoint that adds a version.
  - `GET /agents/{agentId}/instructions` is for humans. Agents never read other agents' instructions.
- **Delivery:** `GET /agent/instructions`, which the guard already uses, returns the shared rules **followed by** the agent's own instructions in a separate, clearly headed section ("## Instructions for @handle"). Same placeholders, same delivery paths (MCP `instructions`, `--append-system-prompt`, Antigravity prompt head).
  - The combined length must fit the delivery limits. `POST /agents` refuses too long a text with a clear message.
- **Security:** the text is written by an admin and shown to an agent, so it must never contain secrets. Show a warning in the editor, and refuse text that matches the existing secret patterns (reuse the guard's or the server's detection, whichever exists). Tests.

## 3. The agent overview pane

The work pane from round 31 (`agent:<agentId>`), opened from an avatar, a mention chip, the agent directory, or the header's activity popover. It runs over `GET /agents/{id}/overview`. Extend it where marked (contract first, additive). Humans get a smaller version (profile, common channels).

- **Header:**
  - the large square avatar with its colour ring, the display name, `@handle`, and the description;
  - chips: state (dolgozik with a pulse / tétlen / offline / szünetel / leállítva / archivált), host (colour dot + name), client + model, and **"Sandbox"** or **"Teljes hozzáférés"** (the latter with a shield icon in `warning`);
  - actions: "DM", "📁 Fájlok", "🖥 Terminálok", "⚙ Beállítások" (opens the agent settings dialog), and for admins a menu: Szüneteltetés / Folytatás, Leállítás (confirmation Dialog), Vészleállítás csak ennek.
- **Sections:** collapsible cards; their open state is remembered per viewer.
  1. **Most:** where the agent works now (thread links), the current live step and the elapsed time (`agent.progress`), and the unacked inbox count with the oldest item's age (**new field** `inbox: {count, oldest_at}`).
  2. **Munkamenetek:**
     - each thread session: channel or thread name (link), context bar `context_tokens / max_session_tokens` (round 30, amber from 80 %), "hamarosan új munkamenet" when near, last run time;
     - sorted by last activity; the 5 latest, then "Összes".
  3. **Felhasználás:** today and the last 7 days (tokens, runs), a bar against `daily_token_limit` when set, and a 7-day sparkline (the dataviz rules of the design system).
  4. **Utolsó futások:** the 5 latest (state icon, start, duration, tokens, `🌐 N`, the thread, and for humans "Hiba részletei" on failures). "Összes futás" goes to the runs page later (round 35); until then, it links to the dev page.
  5. **Terminálok:** the round 31 list for this agent.
  6. **Konfiguráció:**
     - root folder, worktree repo, the folder grants with read/write badges, preview and web_read flags, approval mode, permission mode, model;
     - a 🔒 and "Létrehozáskor rögzítve" note;
     - "Részletek" opens the settings dialog on its Konfiguráció tab.
  7. **Együttműködés:**
     - active delegation grants with expiry and revoke (admins);
     - the agent's **active leases** (resource, expiry, "Felszabadítás" for humans);
     - its **open questions** (`Q-12`, to whom, deadline, linking to the questions Sheet once round 34 has it);
     - common channels.
     - The dev page shows the leases and questions too; take the data from the overview if it has them, otherwise add them to `AgentOverview` (additive).
  8. **Jegyzetek:** the 3 latest notebook entries, with "Megnyitás a Jegyzetek ablakban" (round 32).
  9. **Utasítás:** the own instructions (first lines) and "Megtekintés".
- **Live:** state, activity, progress, run and terminal events update the pane without refetching it all. Refetch the overview at most every 15 s otherwise.
- **Phone:** full screen in the navigation stack; the sections stay collapsible.

## 4. The agent directory and the creation dialog

### `/app/agents`: the directory

- **Search** (Norbi): one field that matches handle, display name, description, host name, client, model and state, with fuzzy matching and the matched part highlighted. `/` focuses it (registered in the round 31 shortcut registry, like every shortcut of this round), and the command palette also finds agents.
- **Filters** (chips, combinable, kept in the URL): host (with colour dots), client (Claude Code / Antigravity), access (Sandbox / Teljes hozzáférés), state (dolgozik / tétlen / offline / szünetel / leállítva), and **"Archivált"** (off by default; archived agents are shown read-only).
- **Grouping:** by host (default; group headers show the host colour, name, online state, agent count), by state, or none. **Views:** cards or a compact table.
  - A card shows the avatar with its ring, the name, `@handle`, state, client + model, access badge, today's tokens, and the last activity.
- Clicking a card opens the overview pane. "⚙" opens the agent settings dialog.

### "Új agent": the detailed creation dialog (admins)

A **large Dialog** (the variant below; a full-screen Sheet on a phone) with a step list on the left and the step on the right. It is the only way to configure an agent, so it holds every setting, with help text.

- **The warning** (Norbi: "figyelmeztessük, hogy utólag nem tudja módosítani, csak kitörölni és újat létrehozni"):
  - a persistent `warning` banner at the top of **every step**: "🔒 Létrehozás után a gép, a kliens, a modell, a hozzáférés, a mappák és az utasítás **nem módosítható**. Változtatáshoz törölni kell az agentet, és újat létrehozni.";
  - a 🔒 next to every immutable field, and an "utólag is módosítható" hint next to the few that are not (name, avatar, colour, description, limits).
- **Steps:**
  1. **Név és profil:** handle (validated live: 2–32, lowercase, free among active agents; if an archived agent had it, say "egy archivált agent neve volt, átveheted"), display name, avatar emoji, colour (swatches + "a gép színe"), description.
  2. **Gép és kliens:**
     - the host from cards (colour, online state, the clients the guard reported with versions);
     - the client among those the host has;
     - model and work model (a list of known models with free text allowed);
     - mode (`direct` / `compact`, one line each on what it means);
     - "Munkafutás engedélyezve" (`allow_work`);
     - permission mode (the client's modes, `auto` by default, a warning on bypass-like modes; **never offer `--dangerously-skip-permissions` as a default**, `CLAUDE.md`).
     - An "API-kulcsos agent (nincs gép)" choice skips the host fields.
  3. **Hozzáférés:**
     - Sandbox or Teljes hozzáférés, as two large choice cards that say what each means (`CLAUDE.md`: a full-access agent runs from its root with Norbi's own settings; a sandboxed one only sees the granted folders);
     - the root folder and the worktree repo (**folder picker**, single mode, part 7);
     - the folder grants (folder picker, multi mode, with read/write);
     - approvals (`none` / `chat`);
     - előnézet and web_read switches, each with its one-line consequence;
     - the host's `guard.yaml`-only settings (`sandbox_network`, `preview_ports`, `allow_unsandboxed`), read-only, with the hint "a gépen, a guard.yaml-ban állítható".
  4. **Utasítás:** the own instructions (optional). Use the round 32 rules editor (split edit/preview, the placeholder bar, the 8 000 counter, the secret warning), and a "Hatályos utasítás" preview: the shared rules plus this text, with the placeholders filled for this handle.
  5. **Keretek és csatornák:**
     - the session limit (`max_session_tokens`, presets 100k / 150k / 300k / nincs) and the daily token limit;
     - **the channels the agent joins** (a multi-select of channels with search, as the dev page's "joinChannels" did);
     - all of these are marked "utólag is módosítható" (memberships later through the channel settings, round 34).
     - `POST /agents` takes the channel list and adds the memberships in the same transaction.
  6. **Összegzés:**
     - two columns: **"🔒 Nem módosítható"** (every configuration value, the folder list with modes, the first lines of the instructions) and **"Utólag módosítható"** (profile, limits, channels);
     - a required checkbox: **"Tudomásul veszem, hogy ezek utólag nem módosíthatók; változtatáshoz törölnöm kell az agentet és újat létrehoznom."**;
     - "Létrehozás" is disabled until the checkbox is ticked. It calls `POST /agents` once. On error, the dialog jumps to the step whose field failed, with the server's message.
- Steps can be revisited freely before creation. Closing with input asks first.
- The token is shown only for an API-key agent, once, with a copy button and a warning.
- **"Másolat új agentként":** the settings dialog of any agent (archived ones too) can open this dialog **prefilled** with that agent's configuration, so "changing" an agent means: copy, adjust, create, then delete the old one. If the handle is still taken by the active original, step 1 says so and offers a new handle or "előbb töröld a régit".

### The agent settings dialog (after creation)

There is **no separate agent page**; the overview pane is the agent's "page". The settings open as a **large Dialog** over whatever is on screen, from the overview pane's "⚙ Beállítások", the "⚙" on a directory card, the host page's agent table, or the command palette ("@handle beállításai"). It is addressable in the URL (`?agent=<handle>&settings=<tab>`).

- **The large Dialog variant** (also used by the creation dialog; add it to `docs/07-design-system.md` and the style guide):
  - `max-w-4xl`, at most 85 vh;
  - a header with the agent's avatar and colour ring, name, `@handle`, host chip and close button;
  - **vertical tabs or steps on the left**, the content scrolling on the right, and a sticky footer;
  - on a phone, a full-screen Sheet where the tab list is the first screen.
- **Stacking:** the folder picker and confirmation Dialogs open **on top of** it. This is an allowed exception to the "one Dialog at a time" rule (one level of child dialog over a large Dialog); write it into the design system. Focus returns to the large Dialog when the child closes.
- **Tabs:**
  1. **Profil** (editable): display name, avatar emoji, colour, description, status text, time zone. Saved with a "Mi változik" summary; unsaved changes ask before switching or closing.
  2. **Keretek** (editable): the session limit and the daily token limit (`PATCH /agents/{agentId}/limits`), with today's usage next to the daily limit.
  3. **Konfiguráció** (**read-only**):
     - every immutable value grouped as in the creation dialog (Gép és kliens, Hozzáférés with the folder list), each with a 🔒;
     - at the top, an `info` banner: "Létrehozáskor rögzítve. Módosításhoz készíts másolatot új agentként, majd töröld ezt.", with the **"Másolat új agentként"** button.
  4. **Utasítás** (**read-only**): the own instructions rendered with the safe markdown, and the "Hatályos utasítás" view.
  5. **Veszélyes műveletek** (admins, a `danger` header):
     - pause or resume, stop, emergency-stop this agent;
     - **"Agent törlése"**: a confirmation Dialog that lists what happens (runs stop, terminals are killed, tokens, folder access and channel memberships end, messages stay, the handle becomes free) and **requires typing the handle** to enable the button.
- **Live:** if an `agent.*` event arrives while the Dialog is open, a banner says "Közben módosult", with "Frissítés"; unsaved profile fields are kept. An archived agent's dialog is entirely read-only, except "Másolat új agentként".

## 5. Machines: `/app/hosts`

- **Host list** (left column, or the top on a phone), one card per host:
  - a colour stripe and the name;
  - **online state** from `last_seen_at` (online under 90 s, otherwise "utoljára 12 perce"), OS, guard version, agent count;
  - the Claude limit ring when reported;
  - warnings: rejected paths, an unwritable exchange folder, an old guard version (older than the server's), a revoked state.
  - "Új gép" at the top.
- **Host detail:** tabs.
  1. **Áttekintés:**
     - state;
     - the reported clients with versions;
     - sandbox (`bwrap` version, or "nincs");
     - `allow_unsandboxed`;
     - the exchange folder and whether it works;
     - roots;
     - Claude limits (5 h and 7 days with reset times);
     - **rejected paths** as a warning list with the agent and the reason.
  2. **Agentek:** the assigned agents (the directory's compact table filtered to this host), and **"Új agent ezen a gépen"**, which opens the creation dialog with this host chosen.
  3. **Mappák:** the roots in the folder picker's browse mode (no selection), with **badges for which agents have a grant where** (avatar stack per folder, read/write). Norbi sees at a glance who can touch what.
  4. **Worktree-k és előnézetek:** the worktrees (agent, path, branch) and the previews (agent, name, link, state, expiry, "Leállítás").
  5. **Beállítások:**
     - name and **colour** (the palette as swatches, live preview on the agents' avatars);
     - the `guard.yaml`-only settings, read-only, with the hint where to change them;
     - **revoke** (danger, confirmation Dialog: "every agent token of this machine ends").
- **"Új gép" wizard:**
  1. "Párosítókód kérése" shows the code with a countdown and the ready-to-copy command:

     ```
     colony guard enroll -server <URL> -code <CODE> -name <GÉPNÉV>
     ```

     The machine name is a field filled in by the wizard.
  2. The wizard **waits live** for the enrollment (WebSocket event), then shows the new host and asks for approval if needed.
  3. Next steps: `colony guard install -config guard.yaml`, a minimal `guard.yaml` example with `roots`, and "Kész": the host page, where the colour can be chosen.
- **Live:** host reports, state and colour changes update the list and the detail.

## 6. Host-level folder listing (server + guard)

The folder picker needs the guard to list folders **before** the agent exists, even on a host with **no agents yet**. Today's file relay (`POST /agents/{agentId}/fs`) is per agent and limited to that agent's grants.

- **New relay:** `POST /hosts/{hostId}/fs` with op `list_dirs` (`path`; empty = the list of roots). Admins only (audited, without paths in the event payload). 15 s timeout, 504 on timeout, `host_offline` when the guard is not connected.
- **Transport:** read `internal/gateway` and `internal/guard` first. The request must reach the guard over a connection the guard holds with its **host token**, not an agent token. If the guard has no such WebSocket connection today, add one (hello with the host token, accepted only for this relay frame type and the host's own requests), and explain the choice in the report. Polling is not acceptable: the picker must answer in about a second.
- **The guard answers only inside its own roots** from `guard.yaml` (the same `MayAccessPath`-style checks as the file browser):
  - **directories only**, no files;
  - symlinks that leave the roots are not followed;
  - at most 500 entries per folder (sorted, with `truncated`);
  - per entry: name, `is_git_repo` (has a `.git`), `has_children`, `hidden` (dot names);
  - **secret-looking folders** (`.ssh`, `.gnupg`, `.aws`, `.config/colonyhall`, and the guard's existing deny list) are listed as `denied: true` and can never be granted;
  - heavy folders (`node_modules`, `.git`, `vendor`, `dist`, …) are listed as `heavy: true` and are collapsed.
- **The server checks paths at creation:** `POST /agents` refuses a root folder, worktree repo or grant outside the host's reported roots. Tests.
- **Guard tests** with a temporary tree: roots only, symlink escape, denied names, truncation, git detection.

## 7. The folder picker

A reusable component for the root folder, the worktree repo and the grants in the creation dialog, and for browsing on the host page. It appears as a large Dialog on desktop (`max-w-3xl`, 70 vh, stacked over the creation dialog) and a full-screen Sheet on a phone.

- **Left column:** the host's roots (from `guard.yaml`, via `list_dirs` with an empty path), each with a drive or folder icon and its path.
- **Right side:**
  - a breadcrumb (each segment clickable);
  - the folder list: lazy tree or column view, Finder-like, switchable;
  - a filter field over the loaded entries.
  - Each entry has an icon (folder, git repo with the branch icon, lock for `denied`, dimmed for `heavy`) and a chevron when `has_children`.
  - Hidden folders are off by default, with a switch.
- **Modes:**
  - **single** (root folder, worktree repo): the worktree mode allows only `is_git_repo` entries;
  - **multi** (grants): a checkbox per folder with a **read/write segmented toggle**, and the selection collected in a tray at the bottom ("3 mappa: 2 olvasás, 1 írás");
  - **browse** (host page): no selection, with grant badges per folder.
  - Selections the dialog already holds (also from "Másolat új agentként") are pre-marked.
  - Nested selections (a folder and its child) are allowed, but the tray warns that the parent's mode already covers the child.
- **States:**
  - loading (skeleton rows);
  - `host_offline`: an EmptyState "A gép most nem elérhető", with a retry and the last-seen time;
  - `denied` entries: disabled, with a tooltip "Titkos mappa, nem adható";
  - timeout: retry.
- **"Útvonal beírása"** as a small secondary link for admins only, a fallback when the guard is offline. The path is validated against the reported roots, and the result is shown as a warning chip.
- Keyboard: arrows, Enter opens, Space toggles selection, Backspace goes up.
- Vitest tests for the selection logic (single, multi, browse, modes, nested warning, git-only, prefill).

## Not in this round

- the agent **version picker** and agent families with versions (later; this round's handle reuse and the `version` column prepare for it);
- an atomic "replace" (create the copy and archive the original in one step);
- the runs page, the settings pages, tokens, audit, the emergency-stop release flow (round 35);
- the chat completeness items (round 34);
- editing `guard.yaml` from the UI (it stays on the machine by design: the server may not widen the roots).

## Docs and report

- Update:
  - `docs/01-terv.md`:
    - "Agent életciklus": the rule stands, now enforced by the server; the limits count as changeable "keretek"; deleting means archiving; the handle can be reused after archiving; own instructions are part of the configuration; the version picker comes later;
    - "Gépek, mappahozzáférés és sandbox": the host-level folder listing, and the picker replacing typed paths;
  - `CLAUDE.md` rule 8 only if its wording needs the "delete and create" note;
  - `docs/02-muszaki-specifikacio.md`: `POST /agents`, the closed endpoints, limits, archive, the partial unique index, `agent_instructions`, colours, the host relay and the guard's host connection;
  - `docs/07-design-system.md`: the colour palette, the agent/host colour rules, the large Dialog with steps or tabs, the immutable-field pattern (🔒, the warning banner, the confirmation checkbox), the folder picker, and the wizard pattern;
  - `docs/06-allapot-es-uzemeltetes.md`: how to change an agent from now on (copy, create, delete);
  - `task.md`.
- In the report, as usual, and also:
  - how many tests moved to `POST /agents`;
  - what changed on the dev page;
  - the guard deploy order;
  - screenshots of:
    - the directory (cards and table, a search with highlights);
    - the overview pane;
    - **every step of the creation dialog, with the warning banner and the summary checkbox** (desktop and phone);
    - each tab of the settings dialog (the read-only Konfiguráció tab with "Másolat új agentként");
    - the delete confirmation;
    - the folder picker (single, multi, offline, denied);
    - the host page tabs;
    - the new-machine wizard.
