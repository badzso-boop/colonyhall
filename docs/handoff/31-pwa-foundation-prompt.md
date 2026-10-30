# Handoff prompt 31: PWA foundation — Tailwind, the Colonyhall design system, logo, app shell and panel layout

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/pwa-foundation ../new-chat-worktrees/pwa-foundation main
cd ../new-chat-worktrees/pwa-foundation
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/31-pwa-foundation-prompt.md)"
```

---

Branch: `feature/pwa-foundation`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/pwa-foundation`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`. Commit only on `feature/pwa-foundation`; never merge, never push. **The PWA scaffold (`feature/pwa-scaffold`, part 1) must be merged first.** This round needs **no migration**; if you find you need one, stop and say why in the report.

After each part: `gofmt`, `make test`, `make lint`, `make test-web`, `make gen-check` if the contract changed. **Commit only when they pass.** Check every screen in a real headless browser at **1440×900, 1024×768 and 390×844** in **both themes**, with no `pageerror` and no console errors; save the screenshots under `docs/handoff/31-screens/` and link them in the report.

## Why (Norbi, 2026-10-30)

The dev page (`internal/httpserver/web/index.html`, 6 200 lines) has reached its limit. The plan always said the final UI is a React + Vite + TypeScript PWA (`docs/01-terv.md`, `docs/02-muszaki-specifikacio.md`, "Felület"). This round lays its foundation and fixes the **design decisions** every later PWA round builds on:

- **Tailwind CSS is the styling base.**
- **A design system** with the colony (hangyaboly) look: **black and red**, a **dark and a light mode**, one font pair, one way to open modals and panels.
- **A logo and an SVG icon.**
- The **Slack-like layout** from `task.md` ("Slack-szerű felület a React PWA-hoz"): grouped sidebar, replies always in threads, the thread in a right column, terminal and file browser panels that never close a thread.

Scope of this round: the foundation, the design system, the shell, the sidebar, the channel feed, threads, the composer with attachments, and the terminal and file browser panes (both read-only, over APIs that already exist). Everything else on the dev page (Gépek, runs, wiki, planner, rules, compaction editing, approvals panel…) moves in later rounds. **The dev page stays at `/` and must keep working;** the PWA is served at `/app/` until it has everything.

## 0. The contract: what is there and what is missing

State on `main` (checked 2026-10-30): all 136 REST operations the server registers are described in `api/openapi.yaml`, and `make gen-check` is clean. **REST needs one additive field this round** (point 4 below). Missing for a TypeScript client:

1. **No TypeScript types are generated.** `make gen` only runs `oapi-codegen`. Add `openapi-typescript` (pinned, as a `web/` devDependency) and generate `web/src/api/schema.d.ts` in `make gen`; extend `make gen-check` to diff it. Wrap `fetch` in a small typed client (`openapi-fetch`, MIT, or a 50-line helper of your own); never write request or response types by hand.
2. **WebSocket frames.** `WsHello`, `WsWelcome`, `WsEvent`, `WsReplayDone` and `WsResyncRequired` are in the spec, but the gateway also sends `agent.activity` and `agent.progress` frames (`internal/gateway/gateway.go`) that are not. Describe them as schemas (`WsActivity`, `WsProgress`) exactly as the server sends them today. `fs.request` goes to guards only; leave it out and say so.
3. **Event payloads are untyped** (`WsEvent.data: object`). For every event type the PWA consumes in this round (at least `message.created`, `message.retracted`, the reaction events, `channel.created`, `channel.member_added`, `channel.member_removed`, the agent state events, the terminal and file events the panes use; take the exact names from the code, since there is no message edit event), add a payload schema named `Event<Name>` and list the mapping event type → schema in the `WsEvent` description. Read the real payloads in the modules' `events.go`. **Do not change what the server sends.** If an e2e test can cheaply check one real payload per schema against the spec, add it.
4. **Thread participants:** the thread summary row (part 5) needs the participants without one request per thread. Add `reply_participant_ids` (up to 3, most recent repliers first) to `Message` (additive; the store query in `memory` and `postgres`; storetest; e2e).

## 1. The `web/` project

**The scaffold already exists (`feature/pwa-scaffold`, merged before this round).** Read it first and build on it; do not recreate it:

- `web/package.json` with exact versions and a lockfile: React 19.3, Vite 8, TypeScript 6.0 (not 7: `typescript-eslint` 8.72 supports TypeScript below 6.1 only), Tailwind CSS 4.3 (`@tailwindcss/vite`), React Router 8, i18next 26 / react-i18next 17, Vitest 5 (jsdom), ESLint 10 (flat config, `typescript-eslint`, `react-hooks`). Use the latest stable releases when you add packages; when a newer major is out, check the peer ranges before upgrading.
- `web/scripts/go-ignore-node-modules.mjs` (postinstall): writes a `go.mod` into `node_modules`, because some npm packages ship Go sources that `go test ./...` would otherwise pick up. Keep it.
- `web/scripts/keep-dist.mjs`: recreates the tracked `web/dist/.keep` after each build.
- `web/embed.go` (`colonyhall/web`, `//go:embed all:dist`) and `internal/httpserver/app.go`: `/app/` with the SPA fallback, 404 for missing files, 503 "run make web" without a build, a year of caching for `assets/`, `no-cache` for the rest, `nosniff`. Tests in `app_test.go`.
- `web/src/`: `main.tsx` (`BrowserRouter basename="/app"`), `App.tsx` (a placeholder page to replace), `i18n.ts` (`colony.lang`), `theme.ts` (`colony.theme`, the same key as the pre-paint script in `index.html`), `styles/tokens.css` (the starting semantic tokens, `.dark` overrides) and `styles/index.css` (`@custom-variant dark` on the `.dark` class), and tests.
- `make web`, `make test-web` (dev page Node tests + typecheck + lint + Vitest), the `node:22-alpine` stage in `deploy/Dockerfile`, `.dockerignore`.

What this part still adds:

- `web/` at the repo root (as `docs/02` says): **Tailwind CSS v4** (CSS-first configuration with `@theme`), i18next with `web/src/locales/hu.json` and `en.json` (Hungarian is the default; no text in components, only keys), React Router.
- **Accessible primitives:** Radix UI (MIT) for dialog, dropdown menu, popover, tooltip, tabs and scroll area. Style them with Tailwind; do not add a component kit with its own look.
- **Embedding, Make, Docker:** done by the scaffold (above). The Docker stage was not built on the dev machine (no Docker there): **build the image once** (`docker build -f deploy/Dockerfile .`) and fix it if needed.
- **PWA basics:**
  - a web app manifest (name, `theme_color`, icons from part 3, `display: standalone`, `start_url: /app/`);
  - a minimal service worker that caches only the app shell (never API responses, never `/api/`);
  - **after a deploy**, a new service worker version shows a toast "Új verzió érhető el" with "Frissítés" (no silent reload while the user is typing);
  - `viewport-fit=cover` and the `safe-area-inset-*` paddings, so the header, the bottom bar and the composer clear the iPhone notch and home indicator.
  - Push notifications are a later round.
- **Licences:** no AGPL or other copyleft dependency. List every runtime dependency with its licence in the report.
- **Monaco:** reuse the vendored Monaco Editor (`internal/httpserver/web/vendor/monaco`, already served) by URL; do not bundle a second copy.

## 2. The design system

Write it down in **`docs/07-design-system.md`** (Hungarian, the binding reference for every later round) and build it as **`web/src/styles/tokens.css`** (`@theme` tokens) plus a living style guide at **`/app/design`** that shows every token and component in both themes side by side.

### Colours: black and red, the colony

- **Character:** an ant colony at night: deep, warm blacks, one strong red (the soldier ant) and nothing loud besides it. Red means "Colonyhall", the primary action, a mention, and danger; use it sparingly so it keeps its weight.
- **Tokens, semantic only** (components never use raw colours): `bg`, `surface`, `surface-raised`, `surface-sunken`, `border`, `border-strong`, `fg`, `fg-muted`, `fg-subtle`, `accent` (red), `accent-hover`, `accent-fg` (text on red), `accent-soft` (red tint for backgrounds), `danger`, `warning`, `success`, `info`, `focus-ring`, `terminal-bg`, `terminal-fg`.
- **Starting values** (tune them, but keep the character):
  - dark: `bg #0b0a0a`, `surface #141212`, `surface-raised #1c1919`, `border #2a2525`, `fg #f2eeee`, `fg-muted #a39a9a`, `accent #e5383b`, `accent-hover #f25c5f`;
  - light: `bg #faf8f7`, `surface #ffffff`, `surface-raised #ffffff`, `border #e6e0de`, `fg #141111`, `fg-muted #6b6262`, `accent #c1121f`, `accent-hover #a40f1a`.
  - `warning` is amber and `success` is green, both desaturated; `danger` may equal `accent` in hue but must be distinguishable by an icon, never by colour alone.
- **Contrast:** WCAG AA (4.5:1 for text, 3:1 for UI borders and icons) for every text/background token pair in both themes. Add a Vitest test that computes the ratios from `tokens.css`, so a later change cannot silently break them.
- **Themes:** `system` (default), `light` and `dark`, with a switch in the user menu. Tailwind `dark:` uses a `.dark` class on `<html>`, set before first paint by an inline script (no flash). Store the choice in `localStorage` (try/catch).
- **The terminal is always dark** (`terminal-bg`, `terminal-fg`), in both themes.

### Typography

- **UI:** **Inter** (variable, SIL OFL). **Code, terminal, file paths, handles in technical places:** **JetBrains Mono** (variable, SIL OFL). Both are **self-hosted** through `@fontsource-variable/*`, with the `latin` **and `latin-ext`** subsets (ő, ű). There is no request to Google Fonts: the server is self-hosted and Tailscale-only.
- **Scale** (rem, 16 px base): 12 (meta, badges), 13 (sidebar, secondary), 14 (**message body, default**), 16 (panel titles), 20 (page titles), 24 (empty states, logo wordmark). Line height 1.5 for text, 1.3 for headings. Weights 400, 500 and 600 only.
- **Numbers that change** (token counts, timers, unread counts) use `font-variant-numeric: tabular-nums`.

### Spacing, shape, depth, motion

- A 4 px spacing scale; radius `sm 4`, `md 8`, `lg 12`, `full`.
- **Shape language:** **humans are circles, agents are rounded squares (`md`)**, everywhere: avatars, mention chips, member lists. This is the main way to tell them apart at a glance; colour is never the only signal.
- **Depth:** borders in dark mode, soft shadows only in light mode; overlays are `black/60` with a light backdrop blur.
- **Motion:** 150 ms ease-out for panels and dialogs, none when `prefers-reduced-motion` is set. The "working" indicator pulses; nothing else animates by itself.

### Components to build now (each in the style guide, each with a Vitest render test)

Button (primary red, secondary, ghost, danger; sizes sm and md; loading), IconButton with a required `aria-label`, Input, Textarea, Select, Checkbox, Switch, Badge (count, status, git letter), Avatar (human circle or agent square, emoji or initials, status dot), Tooltip, DropdownMenu, Tabs, Dialog, Sheet, Toast, EmptyState, Skeleton, and **HoldButton** (press and hold for 2 s, with a progress ring; for the emergency stop, `docs/01-terv.md` "Vészleállító").

**Icons:** Lucide (`lucide-react`, ISC) for the UI icons, 16 px in dense places and 20 px elsewhere, `stroke-width 1.75`. The Colonyhall logo is our own (part 3).

### How dialogs and panels open (the rules)

There are exactly four kinds of overlay. Pick by purpose, never by taste:

1. **Dialog** (centred, `max-w-md` or `max-w-lg`): short forms and confirmations (new channel, members, rename, "really kill this terminal?"). Esc and a click on the overlay close it (except while a form has unsaved input, where Esc asks first). Focus is trapped and returns to the trigger afterwards. **On a phone (< 640 px) every Dialog becomes a bottom Sheet** with a drag handle.
2. **Sheet** (from the right on desktop, from the bottom on a phone): longer read-only detail that relates to the current screen (message details, run log, error details for humans, approval details).
3. **Pane** (part 5): a persistent column in the layout: thread, terminal, file browser, agent overview. It is not modal: the rest of the app stays usable. Panes are addressable in the URL.
4. **Toast** (bottom right on desktop, top on a phone): feedback that needs no answer ("Visszavonva", "Feltöltve"), with an optional single action (e.g. "Visszavonás" for 10 s).

A fifth kind, the **floating window** (a movable, minimisable, translucent in-page window for tasks, the calendar and notes), is specified and built in round 32. Leave its layer in the z-order: panes < floating windows < Sheet and Dialog < Toast.

Rules: at most **one** Dialog or Sheet at a time; the only exception is a confirmation Dialog on top of a Sheet. **No `window.confirm/alert/prompt`.** Destructive actions use a danger Button inside a confirmation Dialog; the emergency stop uses HoldButton and no dialog. Every overlay is a Radix primitive.

## 3. Logo and icon

- **Concept:** a minimal, geometric **ant** seen from above: three segments (head, thorax, abdomen) as circles or ellipses on a vertical axis, two short antennae and three pairs of legs as single strokes. It must read at 16 px. Red on black is the primary form. Optionally the ant sits inside a soft, mound-like rounded shape (the "hall"). No bees, no hexagons.
- **Files** (hand-written SVG, `viewBox="0 0 24 24"` for the mark, no embedded raster, each under 2 KB, `currentColor` where it makes sense):
  - `web/public/brand/mark.svg` (the ant), `mark-mono.svg` (single colour), `logo.svg` (mark + "Colonyhall" wordmark in Inter 600; the text converted to paths or kept as text with a fallback, explain which in the report);
  - `favicon.svg`, and PNG `icon-192.png`, `icon-512.png`, `maskable-512.png` (safe zone respected) and `apple-touch-icon.png`, generated from the SVG by a small reproducible script in `web/scripts/` (commit both the script and the PNGs).
- Show every variant on `/app/design` on dark, light and red backgrounds. Also put `favicon.svg` on the dev page.

## 4. App shell and the sidebar

### Layout grid

| Width | Columns |
| --- | --- |
| ≥ 1280 px | sidebar (260 px, resizable 220–360) · main · right pane (440 px, resizable 360–720) |
| 768–1279 px | sidebar as a collapsible rail (icons and unread dots) or a drawer · main · right pane as an overlay drawer over main |
| < 768 px | one screen at a time (a navigation stack, part 5) and a bottom bar |

### The header (Norbi, 2026-10-30: "a headert is ki kell találni")

One 48 px bar on `surface` with a bottom `border`, the same on every page. From left to right:

1. **Sidebar toggle** (☰; on desktop it collapses the sidebar to the rail; it also opens the sidebar drawer while the left slot holds a work pane).
2. **The logo mark and the workspace name** (Inter 600, 16 px). It links to the last open conversation.
3. **Breadcrumb of the current place** in `fg-muted`: `# general › szál: "Release 1.4"`, or the page name (`Wiki › Telepítési jegyzetek`). It is truncated in the middle and the full text is in a tooltip.
4. **Search / command palette** in the centre: a field-looking button "Keresés vagy parancs… Ctrl K". This round it opens a palette that **jumps** to conversations, agents and pages (client-side fuzzy match over what is loaded); message search comes later.
5. On the right, icon buttons with tooltips and count badges:
   - **"❓ Válaszra vár"** with the open question count (red badge when > 0);
   - **activity:** "⚡ N dolgozik" when agents are working, with a popover listing who works where (from `agent.activity`), each row linking to the thread;
   - **Claude limit meter** (admins, when the guards report it, round 27): a small 5-hour ring, amber from 80 %, with the details ("5 óra: 72 % · újraindul 18:00 · heti 59 %") in a tooltip;
   - **the floating window dock** (round 32 fills it; this round reserves the place);
   - **"Mind áll" HoldButton** for admins (`docs/01-terv.md`), red outline, label hidden below 1280 px;
   - **the user menu** (circle avatar): profile, theme (Rendszer / Világos / Sötét), language, log out.
- **Emergency state:** while the emergency stop is active, the whole header turns `accent` with `accent-fg` text, "Vészleállítás aktív" in the centre and a "Feloldás" button (re-authentication, `docs/01-terv.md`). The release Dialog with the password comes in round 35; until then "Feloldás" links to the dev page. A halted user (`/me.halted`) sees a thin amber strip under the header.
- **Phone:** a 48 px header with back (or the logo on the list screen), the title of the current screen, and the avatar. The search, the questions and the activity move to the bottom bar.

- **Connection state:** while the WebSocket reconnects, a thin bar under the header says "Újracsatlakozás…" (after 3 s, not on every blip); after a `resync_required` the views refetch quietly. When the session expires (401), the app goes to `/app/login` and returns to the same URL after login.
- **Document title and favicon:** the tab title shows the unread mention count ("(3) Colonyhall"), and the favicon gets a small red dot while there are unread mentions.
- **Phone bottom bar:** Beszélgetések · Válaszra vár · Agentek · Több. Until round 33 builds the agent directory, "Agentek" lists the agents with their state and opens the overview pane. The emergency stop sits in "Több" and as a HoldButton at the top of the conversation list.
- **Pages in the sidebar:** above the conversation groups, a compact "Munkaterület" list of pages (Wiki, Feladatok, Naptár, Ütemezések, Foglalások, Szabályok, …). This round adds only the entries whose page exists. Round 32 adds the rest and their pop-out buttons.

### Sidebar groups (Norbi, `task.md`)

From top to bottom:

1. **The user's own sections** (`/sidebar/sections`, already in the API), in their own order, collapsible. A conversation placed in one of them is not shown again in the groups below.
2. **Csatornák:** public and private channels. Glyph `#` (or a lock icon for private), name.
3. **Közvetlen üzenetek:** DMs with **humans only**. A round avatar, then the display name. (The API has no presence for humans; do not invent one.)
4. **AI-beszélgetések:** DMs with at least one agent, in two subgroups:
   - **Te és az agentek** (human–agent): a square agent avatar with its state (working: a red pulsing dot; idle: grey; offline: a hollow ring; paused or stopped: a pause or stop icon), then `@handle`;
   - **Agentek egymás közt** (agent–agent; admins see them): two overlapping square avatars, the label `@a ↔ @b` in `fg-muted`, and an eye icon with the tooltip "Csak megfigyelés" when the viewer is not a member. If a delegation grant is active between the two agents, show 🤝 with its expiry in the tooltip.
- **How to classify:** a channel with `kind: dm` is classified from `member_ids` and each member's `ActorKind` (from `/actors`). Do this in one pure function, `classifyConversation`, with unit tests (human–human, human–agent, agent–agent, a DM with yourself, a member that is no longer listed). No server change is needed.
- **Row states:** unread makes the name bold; mentions add a red count badge (there is no muting in the API yet, so no muted state); the active row has an `accent-soft` background and a 2 px red left bar. Each group shows its unread total when collapsed. Collapsed state is per viewer (`localStorage`).
- Group headers have a "+" action (new channel, new DM, new DM with an agent). The dialogs behind them come in round 34; until then show the buttons only for actions that already work in the PWA, and link the rest to the dev page.

## 5. Feed, threads and the pane manager

### Channel feed

- Reuse the logic, not the DOM, of the dev page: port `format.js` (dates, day separators, `sessionContext`), `markdown.js` and `mention.js` to typed modules under `web/src/lib/`, **keeping their tests** (convert them to Vitest). The safe markdown rules still hold: build React elements, never `dangerouslySetInnerHTML`, no remote images.
- Message: avatar (circle or square), display name, `@handle` in mono for agents, time (with the date as in round 30), body, attachments, reactions, hover actions (react, reply in thread, copy link, more).
  - Messages that mention the viewer get an `accent-soft` background.
  - Long messages collapse with "Több" / "Kevesebb" (round 17).
  - Code blocks have a language label and a copy button.
  - Image attachments open in a lightbox (Esc closes, arrows step through the message's images, a download button); other files are chips with type icon, name and size.
- **History and read state:**
  - the feed loads the latest page and **loads older pages when scrolling up** (the existing cursor), keeping the scroll position;
  - on opening a conversation it jumps to the first unread message with an "Új üzenetek" divider;
  - while the viewer is at the bottom and the tab is visible, it marks the conversation read (`/channels/{channelId}/read`, debounced);
  - when new messages arrive while scrolled up, a "↓ N új üzenet" pill appears.
- **Permissions:** where the viewer cannot write (`can_write` false, or the `observer` role), the composer is replaced by a muted "Csak olvasási jogod van" bar, and write actions are hidden.
- **Slash commands:** typing `/` at the start of the composer opens a menu of the commands the server already understands (`/schedule`, `/daily`, `/cron`, `/answer`, …: list them from the code), each with a one-line description and its argument hint. Selecting one inserts it. The commands keep working as plain text.
- **Replies always go to the thread** (`task.md`): in a channel, the main feed shows only root messages. Under a root with replies there is a summary row: "N válasz" + the last reply's relative time + up to 3 participant avatars (from `reply_count`, `last_reply_at` and `reply_participant_ids`, part 0). Clicking it opens the thread pane. DMs keep their thread behaviour from the dev page.
- The "dolgozik…" line above the composer (from `agent.activity`) and the live steps (from `agent.progress`) as on the dev page.

### The pane manager (the core rule of this round)

Two **slots** beside the main column: **right** and **left**. The left slot is where the sidebar normally is. Two kinds of panes:

- **conversation pane:** a thread;
- **work panes:** terminal, file browser, agent overview. This round's agent overview is a basic version over the existing `/agents/{id}/overview` (header, state, sessions with the context bar, terminals); round 33 completes it.

Rules:

1. A pane opens in the **right** slot when it is empty.
2. If a **thread** is open on the right and a **work pane** is opened (from the thread, from a chat link, from the composer's buttons), the thread stays. **The work pane opens in the left slot:** it replaces the sidebar, and the left column widens to the work pane's width (default 560 px, resizable). Closing it brings the sidebar back at its previous width.
3. If a work pane is on the right and a **thread** is opened, the thread takes the right slot and the work pane moves to the left slot, keeping its state (scroll position, open file, selection).
4. There is **one work pane at a time, with tabs**: opening another terminal or file opens a new tab in the existing work pane (up to 8 tabs, then the oldest unpinned one closes). Each tab shows an icon, the agent and the name; middle-click or × closes it.
5. Opening a second thread replaces the first thread (no thread tabs).
6. While the left slot holds a work pane, a sidebar toggle button in the top bar opens the sidebar as an overlay drawer, so navigation still works.
7. **768–1279 px:** only one slot exists: the latest pane wins, and the other is one tap away through a pane switcher in the pane header.
8. **< 768 px:** a navigation stack: conversation list → conversation → thread → work pane, each full screen, with a back button and the swipe-back gesture. The composer and the "Fájlok" and "Terminál" buttons stay reachable.
9. **The state lives in the URL**, e.g. `/app/c/:channelId?thread=:rootId&work=terminal:agentId:name,files:agentId:path&tab=1`, so reload, back and forward, and (later) push notifications open exactly this view.

Implement the rules as a **pure reducer** (`web/src/layout/panes.ts`: state + actions → state, and state ↔ URL), with Vitest tests for each numbered rule above at each width class. The components only render what the reducer says.

### Composer and attachments

- A multi-line textarea (Enter sends, Shift+Enter is a new line; on a phone, Enter is a new line and there is a send button), the `@` autocomplete from `mention.js`, and the reply pre-fill from round 26.
- Buttons under the textarea: **"+" (Csatolás)**, **"📁 Fájlok"** (opens the file browser work pane for the channel's agent; in a channel with several agents, the agent picker from the dev page), **"🖥 Terminál"** (opens the terminal list of that agent in the work pane), and send.
- **Attaching files: three ways**, all to the existing `POST /channels/{channelId}/files`:
  1. "+" opens a small menu: "Fájl feltöltése" (the native picker, several files allowed), and on a phone also "Fénykép vagy kamera" (`accept="image/*" capture`). Build the menu from a small registry of entries, because round 32 adds "Ütemezés" to it;
  2. **drag and drop** anywhere over the conversation (or the thread pane, which then attaches to the thread): a full-area drop overlay "Engedd el a csatoláshoz";
  3. **paste** an image from the clipboard into the textarea.
- **The tray:** before sending, attachments sit in a tray above the textarea as chips: a thumbnail for images or a file-type icon, the name (middle-truncated), the size, a progress bar while uploading, × to remove, and a retry button on failure. Uploading starts immediately; send waits for running uploads. Files over the server limit are rejected in the client with a clear message; if no endpoint exposes `COLONY_MAX_UPLOAD_MB`, rely on the server's 413 and say so in the report (no contract change for this alone).
- **Line references** from the file browser (`@handle:path:42-58`) are inserted into the composer as chips, as on the dev page.

## 6. The terminal pane (read-only, existing APIs)

Over the existing relay operations (`term_list`, `term_output`, `term_kill`, `docs/02` "Agent-terminálok"):

- **List view** (the tab "Terminálok · @agent"): a row per terminal with a status badge (fut ⏳ / kész ✅ exit 0 / hiba ⚠️ exit N / leállítva ⛔), the command (mono, truncated), started time, duration (tabular-nums) and a link to its thread.
- **Output view:** a header with agent, name, status, host and elapsed time; below it the output on `terminal-bg` in JetBrains Mono 12.5 px, line height 1.45, rendered as **text only** (React text nodes; the guard already strips ANSI). It refreshes every 2 s while running and stops when done. It autoscrolls; scrolling up pauses autoscroll and shows a "↓ Ugrás a végére" pill. Copy buttons for the output and for the SSH line (`tmux -L colonyhall attach -r -t colony-<handle>-<name>`, with the host name).
- **Admin "Leállítás":** a danger button with a confirmation Dialog. Non-admins do not see it, and the server already refuses them.
- There is no input into the terminal (an interactive web terminal is out of scope, `task.md`).

## 7. The file browser pane (read-only, existing APIs)

Over `POST /agents/{agentId}/fs` (`list`, `search`, `git_status`) and `file-fetch` (`docs/02` "Fájlböngésző"):

- **Header:** an agent picker, the root, branch name, ahead/behind, a search field, and a refresh button.
- **Desktop and tablet:** the tree on top (resizable split) or on the left when the pane is wider than 720 px; the viewer below or on the right.
- **Tree:** lazy expansion; folder and file icons; git badges as small mono letters on a tinted chip (M amber, A green, D red, ? grey, R blue); heavy folders collapsed with a count; a dot on folders with changes; keyboard navigation (arrows, Enter, `/` to search).
- **Viewer:** the vendored Monaco, read-only, with two custom themes, `colony-dark` and `colony-light`, built from the tokens (switching follows the app theme). Markdown has a Formázott/Nyers switch; images have a preview; files over 2 MB show the head and tail views; line wrap is a toggle.
- **Line selection:** selecting lines shows "Hivatkozás beszúrása" (`@handle:path:42-58` into the composer of the conversation in the main column, or of the thread if the thread pane has focus) and "Link másolása".
- **Phone:** the tree full screen; tapping a file opens the viewer full screen with a back button.
- **Errors** (`host_offline`, `timeout`, `denied`, …): an EmptyState with the i18n message and a retry button, never a raw error.

## 8. Keyboard shortcuts and their help (Norbi, 2026-10-30)

- **One registry** (`web/src/lib/shortcuts.ts`) where every shortcut is declared once: keys (with the Mac variant: ⌘ for Ctrl), the scope (global, conversation, composer, pane, page), the i18n description, and the handler. Components register their shortcuts there and never add ad-hoc `keydown` listeners. Later rounds (the calendar's M/W/D, the directory's `/`, the floating windows' Alt+arrows) register into the same place.
- **The help:** `?` (outside text inputs) and the user menu's "Billentyűparancsok" open a Dialog that lists the registered shortcuts grouped by scope, with `<kbd>` styled keys and a filter field. Shortcuts of the current screen come first. On a phone the entry is hidden.
- **Shortcuts this round:**
  - Ctrl K: command palette;
  - `?`: help;
  - Alt ↑/↓: previous/next conversation in the sidebar;
  - Alt Shift ↑/↓: previous/next unread conversation;
  - Esc: close the topmost overlay or pane;
  - `r` on a focused message: reply in thread;
  - `e`: react;
  - Ctrl Shift F: file browser;
  - Ctrl Shift T: terminals;
  - in the composer: Enter / Shift Enter (as part 5), and ↑ in an empty composer focuses the last message.
  - Check for clashes with browser and OS shortcuts and say which you avoided.
- Tests: the registry refuses duplicate keys in the same scope; the help Dialog lists what is registered; shortcuts do not fire inside text inputs unless declared for the composer.

## 9. Data and live updates

- One WebSocket client (`web/src/api/ws.ts`): hello with `last_seq`, replay, `resync_required` → refetch, reconnect with backoff. It dispatches typed events (part 0) into a small store (Zustand, MIT, or React context + reducer; pick one and say why). REST reads through the typed client; TanStack Query (MIT) is allowed for caching.
- Log in at `/app/login` with the existing session cookie flow (`/auth/login`, `/auth/session`).

## Not in this round

Push notifications, the Gépek/runs/wiki/rules/planner/calendar/questions/compaction panels, message search, passkeys, an interactive web terminal, retiring the dev page. Leave a short list in the report of what the next PWA round should move first (Norbi decides).

## Docs and report

- `docs/07-design-system.md` (Hungarian): everything from part 2, the pane rules of part 5, the sidebar groups of part 4, and screenshots.
- Update `docs/02-muszaki-specifikacio.md` ("Felület": Tailwind, Radix, fonts, the `/app/` path, embedding, `make web`), the "Parancsok" table in `CLAUDE.md` (`make web`), and `task.md` (the Slack-like UI item: what is done and what is left).
- In the report, as usual, and also: the dependency list with licences, the bundle size (gzip) of the first load, the contrast table from the test, and the screenshots (three widths, both themes; the sidebar with all four groups; a thread with a work pane in the left slot; the attach tray with an upload in progress; the terminal and file browser panes; the keyboard shortcut help; the logo variants).
