# Handoff prompt 32: PWA pages — floating windows, tasks, calendar, notes, schedules, wiki forum, leases, agent rules

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/pwa-pages ../new-chat-worktrees/pwa-pages main
cd ../new-chat-worktrees/pwa-pages
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/32-pwa-pages-floating-prompt.md)"
```

---

Branch: `feature/pwa-pages`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/pwa-pages`. Never work in `/mnt/g/Projects/new-chat` itself, never touch `main`; commit only on this branch, never merge, never push. **Round 31 (`feature/pwa-foundation`) must be merged first:** it has the `web/` project, the design system (`docs/07-design-system.md`, `tokens.css`, components), the header, the pane manager and the composer. **Read `docs/07-design-system.md` and follow it.** If something here contradicts it, the design system wins, and you note the conflict in the report. No migration is expected; if you need one, take the next free number.

After each part: `gofmt`, `make test`, `make lint`, `make test-web`, and `make gen-check` when the contract changed. Commit only when they pass. Check every page in a headless browser at 1440×900, 1024×768 and 390×844, in both themes, with no console errors, and save the screenshots under `docs/handoff/32-screens/`.

## Why (Norbi, 2026-10-30)

The dev page has a panel for each of these, but they are cramped and some need typing (a schedule is written as `/schedule …` or a cron expression). Norbi wants:

- **wiki:** forum-like;
- **leases (foglalások):** a detailed table;
- **schedules:** an intuitive builder he clicks through, also from the composer's "+" menu;
- **agent rules:** a separate, cleaner page;
- **tasks and calendar:** each with its own menu entry and full page (the calendar with **month, week and day** views), and **each can pop out into a floating window inside the page**, not into a new browser window. The same goes for **notes**.

## 1. Floating windows (the fifth overlay kind)

A generic `FloatingWindow` component and a **window manager**, built as a **pure reducer** (`web/src/layout/windows.ts`) with Vitest tests, like the pane manager. It looks and behaves like a Windows window inside the page.

- **Who has one:** Feladatok, Naptár and Jegyzetek. At most one window of each kind; several kinds may be open at once.
- **Opening:**
  - the ⧉ "Kiugrasztás" button in the page's own header;
  - the ⧉ icon on hover next to the page's entry in the sidebar's "Munkaterület" list;
  - the command palette ("Feladatok kiugrasztása").
  - Opening it from the full page navigates back to the last conversation, so the window floats over the chat.
- **The title bar** (32 px, `surface-raised`, drag handle): the icon, the title, and on the right, Windows-style, four icon buttons with tooltips and `aria-label`s:
  1. **Halványítás** (opacity, see below);
  2. **Kis méret** (minimise: — );
  3. **Teljes oldal** (□): navigates to the full page (`/app/tasks`, `/app/calendar`, `/app/notes`) and **closes the floating window**;
  4. **Bezárás** (×).
  - Double-clicking the title bar also opens the full page.
- **Moving and resizing:**
  - Drag the title bar anywhere in the viewport. The window is clamped so its title bar always stays reachable, and it snaps to the edges within 16 px.
  - Resize from all edges and corners; minimum 280×200, maximum the viewport.
  - Keyboard: with the title bar focused, Alt+arrows move it and Alt+Shift+arrows resize it, in 16 px steps.
  - Clicking a window brings it to the front.
  - Register every keyboard shortcut of this round (window moves, calendar M/W/D and arrows) in the round 31 shortcut registry, so they appear in the `?` help.
  - Z-order (round 31): panes < floating windows < Sheet and Dialog < Toast.
- **Translucency** (Norbi: "ha eljön róla a kurzor, legyen áttetsző, de gombnyomásra is lehessen állítani"). Each window has an opacity mode, and the Halványítás button cycles through them:
  - **Normál:** always opaque.
  - **Áttetsző** (the default): opaque while the cursor or focus is on it; **when the cursor leaves, it fades to 55 %** (200 ms).
  - **Halvány:** fades to **20 %** when the cursor leaves. For when Norbi wants to see the chat behind it. The title bar keeps a 1 px `border-strong` outline, so the window can be found again.
  - The button's icon shows the current mode.
  - Hovering or focusing always brings the window back to 100 %.
  - The fade is instant when `prefers-reduced-motion` is set.
  - Faded windows still take clicks; nothing becomes click-through.
- **Minimised:**
  - The window shrinks to a **48 px circle** with the window's Lucide icon (Feladatok: `ListChecks`, Naptár: `CalendarDays`, Jegyzetek: `NotebookPen`) on `surface-raised`, a 1 px `border-strong` ring, and a red ring while it has news.
  - A small badge on the circle: Feladatok shows the number of started tasks; Naptár shows the minutes to the next event when it is within 60 minutes ("12p"), otherwise nothing; Jegyzetek shows a dot for new notes in the current thread.
  - The circle can be dragged anywhere and snaps to the nearest screen edge.
  - Clicking it restores the window at its previous position and size.
  - The circle follows the same opacity mode as its window.
  - The header's window dock (round 31) also lists the open and minimised windows, so a lost one can be brought back.
- **Persistence:** position, size, state (open, minimised or closed) and opacity mode per window, per viewer, in `localStorage` (try/catch). After the viewport shrinks, clamp them again.
- **Widths:**
  - ≥ 768 px: floating windows as above.
  - **< 768 px: there are no floating windows.** Pop-out opens the content in a bottom Sheet (with a ½ and a full-height detent), and minimise is not offered.
- **Accessibility:** `role="dialog"` with `aria-modal="false"`, a labelled title, and focus is never trapped. Esc in a window returns focus to the page; it does not close the window.
- **One content component per feature:** the page and the floating window render the same component, which has a **compact layout below 480 px** of width (e.g. the calendar becomes an agenda). There is no second implementation.
- **Tests:** reducer tests for open, focus and z-order, minimise and restore, clamping after a resize, persistence round trip, the "Teljes oldal" close, one window per kind, and the phone fallback. Component tests for the opacity modes (hover in and out) and the title bar buttons.

## 2. Navigation: pages in the sidebar

The "Munkaterület" block of the sidebar (round 31) gets every page, each with a Lucide icon:

- Feladatok ⧉, Naptár ⧉, Jegyzetek ⧉ (⧉ = pop-out on hover);
- Ütemezések, Wiki, Döntések (a link to the decision log, read-only, as on the dev page), Foglalások;
- admins only: Szabályok.

Routes: `/app/tasks`, `/app/calendar`, `/app/notes`, `/app/schedules`, `/app/wiki`, `/app/wiki/:slug`, `/app/leases`, `/app/rules`. Pages use the main column, and the pane manager still works beside them (a thread can be open next to the wiki). The active page is highlighted like an active conversation. The header breadcrumb shows the page.

## 3. Feladatok (tasks)

Over the existing `/planner/tasks` API (round 28: humans only, the owner's tasks; events `planner.task_*`):

- **Page:** two views with a switch, remembered per viewer:
  - **Lista:** groups Elkezdve, Ütemezve, and a collapsed Befejezve (last 14 days), as on the dev page;
  - **Tábla:** three columns with the same groups; dragging a card between columns sets its status, and dragging within a column sets its position.
  - Both views:
    - a quick-add row ("Új feladat…", Enter adds it as `scheduled`);
    - a card shows the title, the first line of the description, and a status chip;
    - clicking a card opens it in a **Sheet** with the title, the markdown description (Formázott/Szerkesztés), the status, and the started/done times;
    - deleting asks in a confirmation Dialog;
    - ↑/↓ buttons are an alternative to dragging (phone, keyboard).
- **Floating window:** the compact list: started first, then scheduled, quick-add at the top, and a status checkbox cycle (click: ütemezve → elkezdve → befejezve).
- **Live:** `planner.task_*` events update both views and the window; changes in another tab appear immediately.

## 4. Naptár (calendar)

Over `/planner/calendars` and `/planner/events`:

- **Page header:**
  - "Ma" button, ‹ › arrows, the current range as a title ("2026. október", "okt. 26 – nov. 1.", "csütörtök, okt. 30.");
  - the view switch **Hónap · Hét · Nap** (keys M, W, D; ← → move);
  - a calendars popover with a colour dot and visibility toggle per subscription, plus the subscription management from round 28 (add, refresh, remove, masked URL, last fetch status);
  - ⧉ pop-out.
- **Hónap:**
  - a 6×7 grid starting on **Monday** (Hungarian locale; follow the UI language);
  - today highlighted with a red circle on the date;
  - all-day events as bars, timed events as "09:30 Cím" rows with the calendar's colour dot;
  - "+N további" opens a day popover;
  - clicking a day opens the Nap view.
- **Hét:**
  - seven day columns, an all-day strip on top, an hour grid (default 07:00–20:00 visible, scrollable 0–24);
  - overlapping events side by side;
  - a red "now" line with a dot.
- **Nap:** the same as Hét for one day, wider event blocks with location.
- **Event detail:** a popover (Sheet on a phone) with the title, time, all-day flag, calendar, location and the description (markdown-safe plain text, links clickable, no remote images).
- **Read-only:** the calendars are subscriptions, so there is no event creation in Colonyhall (say so in an EmptyState hint).
- **Floating window:** compact: a mini month on top and the agenda below (today and the next 7 days, grouped by day, all-day first). Clicking a date in the mini month scrolls the agenda.
- **Phone:** Hónap shows dots instead of titles, and tapping a day shows its agenda under the grid. Hét is replaced by a 3-day view. Nap stays.
- **Server changes** (contract first):
  - `/planner/events` today allows **at most 14 days**, and the cache holds now − 1 day … now + 60 days. The month view needs 42 days (6 weeks) and going back a little. Raise the limit to **42 days**, and widen the cached window to **the first day of the previous month … now + 120 days**.
  - Update the round 28 tests and add tests for both new edges (a 42-day window is accepted, a 43-day one is refused).
  - Note the larger cache in the report.

## 5. Jegyzetek (notes)

The agents' thread notebooks (`/agents/{id}/notes`, humans only, read-only for humans; `note_get`/`note_set` are the agents' tools):

- **Page:** a list on the left, by agent and then by thread (thread title, last note time), and the selected thread's notes on the right, newest first, rendered with the safe markdown. A search over the loaded notes. Each note links to its thread (opens the thread pane).
- **Floating window:** **follows the active thread**. It shows the notes of every agent that has written in the thread open in the main column or the thread pane. A 📌 "Rögzítés" button pins it to the current thread so it no longer follows. With no thread open, an EmptyState: "Nyiss meg egy szálat".
- **Live:** if no event announces a new note, refetch when the active thread changes, when the window gains focus, and every 30 s while visible. Do not add an event only for this; if one already exists, use it.

## 6. Ütemezések (schedules): a builder, no cron typing

Over `/schedules` (create, list, cancel; `at` for one-off, `cron` + `timezone` for repeating, `catch_up`), events `schedule.*`:

- **The builder** is one Dialog (a Sheet on a phone), used both from the Ütemezések page ("Új ütemezés") and from the **composer's "+" menu → "Ütemezés"** (registered in the round 31 registry). Opened from the composer, it is prefilled with the composer's text and the current channel or thread, and the composer is cleared after saving.
  1. **Mit?** The message (a textarea with the `@` autocomplete), and where it goes: the channel or thread picker, prefilled.
  2. **Mikor?** A segmented control:
     - **Egyszer:** a date picker (calendar popover) and a time picker (15-minute steps, free typing allowed). Quick chips: "Ma 18:00", "Holnap 09:00", "Hétfő 09:00", "1 óra múlva".
     - **Naponta:** a time; "csak munkanapokon" switch.
     - **Hetente:** weekday chips H K Sze Cs P Szo V (multi-select) and a time.
     - **Havonta:** the day of the month (1–31, or "utolsó nap" if the server's cron supports it, otherwise leave it out and note it) and a time.
     - **Haladó:** a raw cron field for admins and power use, with validation.
  3. **Időzóna:** default `Europe/Budapest` (or the user's profile time zone), changeable in a searchable select.
  4. **Ha a szerver épp nem futott:** "Pótolja egyszer" / "Kihagyja" (`catch_up`), with a one-line explanation.
  - **Live summary under the form,** in plain Hungarian: "Minden hétköznap 09:00-kor (Europe/Budapest) a #general csatornába", and **the next 5 run times**. The run times come from the server (below), so the builder and the server never disagree.
  - The generated cron is shown read-only in small mono text under the summary, for the curious.
- **Server change (contract first):** `POST /schedules/preview` with the same body as create. It returns `{ summary_cron, next_runs: [date-time × 5] }` or a validation problem. Use the same parser and time zone logic as create (`robfig/cron/v3`, already used). Humans only, no side effects, no event. Add e2e tests: valid daily, weekdays, a DST change (the last Sunday of October in Europe/Budapest), an invalid cron, and an agent token → 403.
- **The client builds the cron** from the segmented choices in one pure function, `buildCron`, with Vitest tests for each mode, including weekday sets and the workdays switch. Saving sends `at` for Egyszer and `cron` for the rest.
- **The page:**
  - a table with: the message (first line), the destination, the repetition in words (from the cron, with the same function in reverse for the shapes the builder makes; otherwise the raw cron), the next run, the last run with a link to the message, the run count, the state, and the last error (humans only, short);
  - filters: Aktív / Kész / Törölt, and admins: Mindenkié;
  - row actions: "Megszakítás" (confirmation Dialog) and "Másolás új ütemezésbe" (opens the builder prefilled). The API has no edit, so the copy-then-cancel flow replaces it; say so in the UI hint.
- **Chat:** `/schedule` and the other slash commands keep working.

## 7. Wiki: a forum

Over `/wiki` (any actor reads, creates and appends; humans edit, delete and restore; agents only extend), events `wiki.*`:

- **The idea:** a page is a **topic**. Its first revision is the opening post, and every **append** is a **reply** (the server already adds the `@handle, time` header to each append). Human edits (`put`) show as a "✎ szerkesztette @norbi" line between posts.
- **Index (`/app/wiki`):**
  - **Tags** act as categories: a tag list on the left (with counts; "Összes" first), chips on a phone.
  - **Topic rows:** title, a two-line excerpt, the tags, the author's avatar (circle or square), the number of posts, up to 3 participant avatars, and the last activity time (relative, with the full date in a tooltip).
  - **Ordering:** sort by last activity (default), by created, or by title.
  - **"Új" marker:** topics updated since the viewer's last visit to that topic (per viewer, `localStorage`).
  - Search with the existing `q`. A "Törölt" filter for humans (shows soft-deleted topics with a "Visszaállítás" action).
  - "Új téma" opens a Dialog: title, tags (chips with autocomplete from existing tags), body (markdown with a preview tab).
- **Topic (`/app/wiki/:slug`):**
  - Posts as cards: avatar, name, `@handle`, time, revision number, and the body in the safe markdown.
  - Each card has "Link" (an anchor to the post) and, for humans, "Idézet" (quotes the post into the reply box).
  - A reply box at the bottom (append) for everyone who may append.
  - Human actions in the topic header: "Szerkesztés" (a full editor, warns that it rewrites the topic), "Előzmények" (a Sheet with the revisions and a diff between two of them), "Törlés" (confirmation Dialog).
  - Decisions (`D-12`) and `Q-12` references in the text become chips, as in the chat.
- **Deriving the posts:** from `/wiki/{slug}/revisions`, the text of an append is the revision body minus the previous revision's body (the server only adds to the end). Do this in a pure function, `wikiPosts(revisions)`, with tests:
  - create + appends;
  - a human edit in the middle (an edit revision becomes the "szerkesztette" line, and later appends are diffed against it);
  - a deleted-then-restored page;
  - a revision whose body does not start with the previous one (fall back to showing that revision as a full "edited" body).
- **Server change (contract first, additive):** `WikiPageIndex` gets `created_by`, `post_count` (the create revision plus the append revisions), `last_author_id` and `participant_ids` (up to 5, most recent first), so the index needs one request. Extend the store query in `memory` and `postgres` and the storetest, and add e2e tests.

## 8. Foglalások (leases): a detailed table

Over `/leases` and `/leases/{id}/release` (humans only), events `lease.acquired`, `lease.extended`, `lease.released`:

- **The table**, sortable by every column, with a sticky header:
  - **Erőforrás:** mono, with a copy button;
  - **Tartja:** the agent's square avatar and `@handle`, linking to the agent overview pane;
  - **Megjegyzés;**
  - **Megszerezve:** relative time, with the full date in a tooltip;
  - **Lejár:** a live countdown, `tabular-nums`, amber under 5 minutes, red when expired but not released;
  - **Hátralévő:** a thin progress bar of the remaining time;
  - **Állapot:** Aktív / Lejárt / Felszabadítva (with who released it and when);
  - **Művelet:** "Felszabadítás" for humans, with a confirmation Dialog naming the resource and the holder.
- **Filters:** Aktív (default) / Előzmények (the last 7 days, all states), a holder select, a free-text filter over resource and note.
- **Live:** `lease.*` events update rows in place. A newly acquired lease flashes once with `accent-soft`, without motion when reduced motion is set.
- **Phone:** the rows become cards with the same fields.
- **Server change (contract first, additive):** `GET /leases?state=active|all&since=` (default `active`). `all` returns released and expired leases since `since` (max 7 days, default 24 h). Extend the store query (memory, postgres, storetest), and add e2e tests including the 403 for agents.

## 9. Szabályok (agent rules): a cleaner page

Over `/agent-rules` and `/agent-rules/{version}` (humans read; editing as today, admins), event `agent_rules.updated`:

- **Layout:** the current version on the main surface; on the right (a Sheet on a phone), a **version timeline**: version number, author, time, and "jelenlegi" on the newest.
- **Reading mode** (the default): the text rendered with the safe markdown. The placeholders `{handle}`, `{admins}` and `{workspace}` are shown as chips with a tooltip explaining what the server fills in.
- **Editing** ("Szerkesztés", admins):
  - a split view, with the markdown editor on the left and the live preview on the right (tabs on a phone);
  - a placeholder bar that inserts the chips;
  - a character counter against the 8 000 limit, amber from 90 %.
  - "Mentés új verzióként" opens a confirmation Dialog with a **diff against the current version** and a note that the agents get it at their next new session or version change (`docs/01-terv.md`).
  - Unsaved changes ask before leaving.
- **"Előnézet @agentként":** a select of agents; the preview fills `{handle}` with that agent, `{admins}` and `{workspace}` client-side, and says in a hint that the server's text is authoritative.
- **Diff:** pick any two versions on the timeline to see a line diff (a small, dependency-light diff library with a permissive licence, or your own line LCS; name it in the report).

## Not in this round

Push notifications; the Gépek, runs, approvals and compaction panels; creating calendar events; editing a schedule in place (the API has none); click-through floating windows; multiple windows of one kind.

## Docs and report

- Extend `docs/07-design-system.md` with the floating window (states, opacity modes, sizes, the phone fallback) and the page header pattern, with screenshots.
- Update `docs/02-muszaki-specifikacio.md` for the API changes (`/schedules/preview`, `/leases?state=`, the `WikiPageIndex` fields, the 42-day calendar window and the cache window), and `task.md`.
- In the report, as usual, and also screenshots of:
  - a floating window in each opacity mode over the chat;
  - the minimised circles;
  - the calendar's month, week and day views (desktop and phone);
  - the schedule builder in each mode, with its next-5-runs summary;
  - a wiki topic with replies and an edit line;
  - the leases table with an expiring lease;
  - the rules diff.
