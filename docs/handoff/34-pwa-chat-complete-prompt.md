# Handoff prompt 34: PWA chat completeness — channel editing and `channel_info`, muting, "Szálaim", questions, approvals, compaction, message elements, sections, profiles

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/pwa-chat ../new-chat-worktrees/pwa-chat main
cd ../new-chat-worktrees/pwa-chat
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/34-pwa-chat-complete-prompt.md)"
```

---

Branch: `feature/pwa-chat`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/pwa-chat`. Never work in `/mnt/g/Projects/new-chat` itself, never touch `main`; commit only on this branch, never merge, never push. **Rounds 31–33 must be merged first.** Read `docs/07-design-system.md` and follow it (the large Dialog with vertical tabs, the folder picker and the colour rules come from round 33). This round **needs migrations** (next free numbers: channel archive, channel mute, thread read state) and **changes the guard** (part 1.4): deploy the server first, then the guards.

After each part: `gofmt`, `make test`, `make lint`, `make test-web`, `make gen-check` if the contract or SQL changed. Commit only when they pass. At the end `make test-pg` uncached and `go test -race ./internal/guard/ ./internal/e2e/`. Every authorization rule needs a test that fails without it. Check every screen in a headless browser at 1440×900, 1024×768 and 390×844 in both themes; save the screenshots under `docs/handoff/34-screens/`.

## Why, and Norbi's decisions (2026-10-30)

After rounds 31–33 the PWA has the shell, the pages, agents and machines. To replace the dev page for daily chat, it still needs the chat features the dev page has, plus **channel editing**, which nobody has today: a channel's description cannot be changed after creation.

Decisions, binding for this round:

1. **Channels are edited by admins only:** description, members, rename, archive. That is today's rule for members; it now covers the rest too. **Agents can never change a channel.**
2. **Agents read the channel description only on demand:**
   - a new `channel_info` MCP tool, available to the channel's members;
   - the prompt only mentions in one line that it exists;
   - the description is **not** injected automatically, and a change does **not** notify the agents.
3. **The channel editor also covers:** **rename**, **archive** (and unarchive), and **removing an agent from a channel stops its running work there**, like a retracted message does.
4. **No message editing.** Retract stays the only correction (round 26). Do not add an edit action.

## 1. Channel editing

### 1.1 Server (contract first)

- **`PATCH /channels/{channelId}`** (admins; agents and non-admins get 403):
  - body: `name` and/or `description` (max 250, empty clears it);
  - the name uses the same validation and uniqueness rule as creation;
  - DMs cannot be renamed (409).
  - Event `channel.updated` with `channel_id`, the changed field names and, when renamed, the old and new name. **No description text in the event**: clients refetch the channel.
- **Archive:**
  - `POST /channels/{channelId}/archive` and `/unarchive` (admins); migration `channel.archived_at` (nullable) and `archived_by`. Events `channel.archived` and `channel.unarchived`. DMs cannot be archived.
  - **An archived channel is read-only for everyone:**
    - sending, reactions, uploads and thread replies are refused (409 `channel_archived`), for agents over REST and MCP too;
    - schedules targeting it fail with that `last_error`;
    - compaction does not run in it.
  - Members and history stay. `GET /channels` hides archived channels unless `?archived=true` (admins). `Channel` gets `archived_at`.
  - **On archive,** the channel's undelivered wake items are acked without waking anybody, and running agent work in it stops (the same mechanism as 1.4).
- **Members:** the existing `POST`/`DELETE /channels/{channelId}/members` (admins) stay. Add a **batch add**: `POST /channels/{channelId}/members` accepts `actor_ids: []` besides `actor_id`, additive. Adding to or removing from a DM is refused (409).
- **Chat trail:**
  - every change writes a **silent** system message into the channel (the `silent` option from round 26, wakes nobody):
    - "📝 @norbi frissítette a csatorna leírását";
    - "✏️ @norbi átnevezte: #régi → #új";
    - "👋 @norbi eltávolította @agent-et";
    - "➕ @norbi hozzáadta: @a, @b";
    - "🗄 @norbi archiválta a csatornát" / "visszaállította";
  - and every change goes to the audit log.
- **Removing a member also:** acks that member's pending delivery items for this channel without waking it (same transaction, outbox). From then on, rule 9 (permission on every call) makes the agent's reads and sends to the channel fail. Add tests that a removed agent can no longer read, send, or use `channel_info` there.
- Tests:
  - e2e for every endpoint, including the 403s (non-admin human, agent) and the 409s (DM, archived);
  - storetest for `archived_at` (memory and Postgres);
  - the silent messages do not create wake items.

### 1.2 The `channel_info` MCP tool

- On the Colonyhall MCP server (`/mcp`), a core read-only tool.
  - **Input:** `channel` (id or `#name`; optional, defaulting to the channel of the agent's current work if the MCP context knows it, otherwise required).
  - **Output:** name, kind, description, archived flag, created time, and the members (handle, display name, kind human/agent).
  - **Only for members of that channel.** Non-members get the usual not-found error without details. An archived channel is still readable by its members.
- **The description is data, not instructions** (`CLAUDE.md` rule 10): return it inside a `<channel_description channel="#name" author_role="admin">…</channel_description>` frame, and the tool description says to treat it as context.
- **Sandboxed agents:** add `channel_info` to the allowlist of the round 29 sandbox MCP bridge (`colony-planner`, `planner.sock`; the guard relays with the agent token) and to the Antigravity `perms`. A full-access agent gets it through its normal Colonyhall MCP.
- **The prompt:** one line in the direct-mode and work prompts' tool list: "`channel_info`: the channel's description and members; read it when asked or when you need the channel's purpose". **Do not** change the shared agent rules text; that is Norbi's.
- Tests:
  - member vs. non-member;
  - after removal;
  - archived;
  - the frame around the description;
  - the bridge allowlist accepts `channel_info` and still refuses everything else.

### 1.3 PWA

- **Channel header:**
  - the name, and the description on one line (truncated, "több" expands it, markdown-safe plain text with links);
  - a member avatar stack with the count (circles for humans, squares with colour rings for agents), which opens the member list;
  - **"⚙"** for admins;
  - in an archived channel, an `fg-muted` "Archivált csatorna" bar replaces the composer; admins also get "Visszaállítás".
- **Channel settings: the large Dialog from round 33**, URL `?channel-settings=<tab>`. Non-admins see a read-only "Csatorna adatai" view (description and members) instead.
  1. **Általános:**
     - the name, validated live;
     - the description: a textarea with a 250-character counter and a preview;
     - the hint "Az agentek a `channel_info` eszközzel olvashatják, ha megkéred őket".
  2. **Tagok:**
     - **List:** search; humans first, then agents grouped by machine colour; each row has avatar, name, `@handle`, state; agent rows show "dolgozik itt" when working in this channel.
     - **"Eltávolítás"** asks in a confirmation Dialog. For an agent the Dialog says: "Ha most itt dolgozik, a futása leáll, a függő feladatai törlődnek."
     - **"Tagok hozzáadása…"** opens a multi-select picker. It searches humans and agents (with the round 33 search, colours and access badges), shows current members as disabled "már tag", and adds them with the batch endpoint.
  3. **Tömörítés:** the channel's threshold (`compact_threshold_tokens`, with the workspace default shown) and "Tömörítés most" (part 4).
  4. **Archiválás** (danger): archive with a confirmation Dialog explaining read-only, the stopped work and that it can be restored.
- **"Új csatorna"** (sidebar "+"): a Dialog with name, public/private, description and initial members (the same picker).
- **"Új DM"**: a picker of humans and agents. A DM with an agent lands in the AI group.
- **Live:** `channel.updated`, `channel.archived`, `channel.unarchived`, and the member events update the sidebar, the header and open dialogs. Removal of the viewer moves them out of the channel with a toast.

### 1.4 Guard: stop work when the agent is removed or the channel is archived

- On `channel.member_removed` for one of its agents, or `channel.archived`, the guard **stops that agent's running decide or work run** whose items or session belong to that channel or its threads. Use the same path as `OnMessageRetracted` (whole process group, `guard.go`, `direct.go`).
- It does **not** post a note: the agent is no longer a member, and the server already wrote the silent system message. It logs the stop.
- A run for other channels continues.
- Terminals the agent started from that channel keep running (they belong to the agent, round 23), but their completion wake is dropped by the server if the agent is no longer a member. Test it.
- Tests with fake runners: removal stops the matching run; an unrelated run keeps going; archive stops all runs in the channel.

## 2. Sidebar sections editing

Over the existing `/sidebar/sections` API:

- create, rename and delete a section (the delete Dialog says the channels go back to their default groups);
- reorder sections and the channels in them **by dragging**, with "Áthelyezés…" in each row's menu as the keyboard and phone alternative (a section picker);
- drag a conversation from a default group into a section, and back out.

The order is optimistic, rolled back with a toast if the server refuses.

## 2a. Muting channels (Norbi, 2026-10-30)

Muting is **per viewer** and only changes what that human is bothered by. It never changes what agents receive or whether they wake.

- **Server** (contract first; migration):
  - `channel_mute` (`actor_id`, `channel_id`, `muted_until` nullable = forever, `created_at`), and `PUT`/`DELETE /channels/{channelId}/mute` (humans, for themselves; body: `until` optional);
  - `Channel` gets `muted` and `muted_until` for the caller;
  - an expired mute counts as unmuted without a job;
  - audit is not needed (a personal preference), but the event `channel.mute_changed` goes to the viewer only, so their other tabs update;
  - storetest and e2e, including that an agent token gets 403 and that one human's mute does not show for another.
- **Effect:**
  - a muted conversation's unread messages do **not** make the row bold and do not count in the group totals or the tab title (round 31);
  - **mentions of the viewer still count** and show the red badge (as in Slack);
  - later push notifications (a later round) will respect the same flag: note it in `docs/01-terv.md`'s notification table.
- **UI:**
  - "Némítás" in the conversation menu and the sidebar row menu, with durations: 1 óra, 8 óra, holnap reggelig, 1 hét, végleg;
  - a muted row is `fg-subtle` with a 🔕 icon (this replaces round 31's "no muted state" note);
  - the channel header shows "Némítva eddig: …" with "Némítás feloldása";
  - optionally a "Némított csatornák" collapsed group at the bottom of the sidebar (a per-viewer switch; default off).

## 2b. "Szálaim" view (Norbi, 2026-10-30)

A Slack-like **"Szálak"** entry at the top of the sidebar (above the pages, with an unread count) that collects the threads the viewer takes part in. A thread is "the viewer's" when they wrote the root, replied in it, or were mentioned in it.

- **Server** (contract first; migration):
  - `thread_read` (`actor_id`, `thread_root_id`, `last_read_at`), written when the viewer has seen the end of a thread (the thread pane open at the bottom, debounced, as the channel read in round 31);
  - `GET /me/threads?cursor=` (humans) returns the viewer's threads, newest activity first. Each item has the root message, the channel, `reply_count`, `last_reply_at`, `reply_participant_ids`, `unread_replies` (replies after `last_read_at`) and the latest 2 replies.
  - Only threads in channels the viewer can still read (rule 9: checked on every call).
  - Paging by cursor.
  - Event: a new reply in one of the viewer's threads already arrives as `message.created`; the client updates the list from it, and refetches on `resync_required`.
  - storetest (memory and Postgres) and e2e: participation by root, reply and mention; unread counting; a thread in a channel the viewer left disappears; an agent token gets 403.
- **The page** (`/app/threads`, also in the phone's "Több"):
  - a list of thread cards: channel name, the root message (author, time, first lines), "N új válasz" in red when unread, the latest 2 replies, participants;
  - "Válasz" opens an inline composer in the card, and "Megnyitás" opens the thread pane next to the list (the pane manager rules hold);
  - a filter: Mind / Olvasatlan;
  - "Mind olvasottnak jelölése".
- **Unread totals:** the sidebar entry shows the number of threads with unread replies. Muted channels (2a) do not count unless the viewer is mentioned.

## 3. Questions: "Válaszra vár"

Over `/questions`, events `question.*`:

- The header button (round 31) opens a **Sheet** with tabs: Nyitott (questions to humans and escalated ones), Megválaszolt (last 7 days).
- Each question card shows:
  - `Q-12`, the asking agent (avatar, colour), the question, a link to its thread, the deadline countdown, the default option when set;
  - **the options as buttons** and a free-text answer.
  - Answering updates the card in place.
- The `Q-12` chip in a message opens the same card in the Sheet.
- `/answer Q-12 <válasz>` works in the composer, with autocomplete of open question numbers.
- The header badge counts open questions live.

## 4. Approvals, delegations and grants in the chat

- **Permission requests** (`[approval:<id>]`, round 05): a card in the message for humans.
  - The card shows the tool name, a one-line summary, and "Részletek", which opens a Sheet with the tool input (already masked by the server; never unmasked in the client).
  - Pending requests show **Engedélyezem / Elutasítom** and the auto-deny countdown.
  - After a decision, the card shows who decided and when.
  - Events `approval.*` update it live.
- **Delegation approvals** (rounds 24–26): the buttons under the source agent's message (no Q chat message), with the state chip afterwards (jóváhagyva / elutasítva / lejárt).
- **The grant bar** (round 25) under the conversation header: "🤝 @a ↔ @b: engedély, lejár 17:25". Admins see "Visszavonás", with an in-page confirmation.

## 5. Compaction

- **The compaction system message:**
  - a card with the summary's first lines and "Összefoglaló megnyitása", which opens a Sheet with the markdown summary;
  - for humans with write access, "Szerkesztés" (the round 21 edit flow) and "Újragenerálás" (confirmation Dialog).
  - States `compaction.created`, `done` and `failed` show live (a spinner card while running, an error card with `[error:id]` for humans on failure).
- "Tömörítés most" in the channel menu and in the channel settings (part 1.3).

## 6. Message elements still missing

Port these from the dev page, typed and tested:

- **Gitea cards** for repo, PR, issue and commit links: title, state chip and repo, linking to the public URL (`CLAUDE.md`, Gitea).
- **Local file links** in agent messages (round 16):
  - "Megnyitás" sends the file-fetch request and shows a pending state;
  - the file opens in a viewer Sheet (Monaco read-only for text, the safe markdown, image preview, head and tail for large files), for humans only;
  - errors show as i18n EmptyStates.
- **Reply routing links** (round 27): "↪ válasz a DM-ben" and "↩ az eredeti szálban", linking both ways.
- **The step drawer** under an agent's answer: collapsed "N lépés · 3:12"; it opens the step list with relative paths.
- **Terminal messages** (round 26): the "Terminál megtekintése" button opens the terminal work pane (round 31).
- **Chips:** `Q-12` (part 3), `D-12` (a decision popover with the text and the rationale), `[error:<id>]` (humans: an error Sheet with the details from `/errors/{id}`; agents' text never shows details), and file line references.
- **Rate-limit notes** (round 27) as a muted system line, with the "⏸ szünetel" badge on the agent's avatar while paused.
- **Retract** (round 26): in the message menu with an inline confirmation, and from the 10 s "Elküldve" toast. Retracted messages show "🗑 Visszavonva".
- **Reactions:** the dev page's set, with the agent-facing meaning in the tooltip (👍 egyetértés, 👎 nem ért egyet).

## 7. Profiles

- Clicking an avatar or a name opens a **profile popover** (a Sheet on a phone):
  - avatar (circle or square with ring), display name, `@handle`, description, status text;
  - time zone with the person's local time;
  - for agents also the state, machine and client, and "Áttekintés" (opens the overview pane);
  - "DM".
- **Own profile** (the user menu → "Profilom"): a Dialog with display name, avatar emoji, description, status text, time zone.

## 8. Who may read tasks and the calendar

Round 32 left this out. On the Feladatok and Naptár pages, a "Hozzáférés" button (owner only) opens a Dialog with the agent picker:

- tasks: `all` (default) or selected agents;
- calendar: none (default) or selected agents.

Use the existing `/planner/access`.

## Not in this round

Message editing (Norbi's decision); admin pages (round 35: runs and usage, settings, tokens, audit, emergency-stop release, channel locking); push notifications; message search.

## Docs and report

- Update `docs/01-terv.md` (channel editing rules, archive, and `channel_info` on demand with Norbi's decisions above), `docs/02-muszaki-specifikacio.md` (the endpoints, events, the migration, the MCP tool, the bridge allowlist, the guard stop), `docs/07-design-system.md` (the channel header, the cards for approvals, questions and compaction), and `task.md`.
- In the report, as usual, and also:
  - the guard deploy order;
  - screenshots of:
    - the channel header and settings (each tab, admin and non-admin);
    - the add-members picker;
    - the removal confirmation;
    - an archived channel;
    - the questions Sheet;
    - an approval card before and after;
    - the grant bar;
    - a compaction card;
    - a Gitea card;
    - a file viewer;
    - the step drawer;
    - the profile popover;
    - a muted channel in the sidebar and its header;
    - the "Szálaim" page with unread threads.
