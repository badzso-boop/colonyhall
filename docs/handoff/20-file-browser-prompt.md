# Handoff prompt 20: file browser for an agent's folder (tree, search, git status, Monaco viewer, line references)

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/file-browser ../new-chat-worktrees/file-browser main
cd ../new-chat-worktrees/file-browser
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/20-file-browser-prompt.md)"
```

---

Branch: `feature/file-browser`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/file-browser` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/file-browser`; never merge, never push.

## Why

Norbi wants to see, from the chat, the folder an agent works in: the whole tree, open a file with syntax highlighting (markdown formatted), see the git status, search file names, and **insert references to files and line ranges** into a message, so he can point an agent at `src/api/server.go:42-58` directly. The plan is in `task.md` ("Tervezendő" → "Fájlböngésző az agent mappájához"); this prompt is its first round, for the dev page (the PWA gets the same API later).

Read first: `CLAUDE.md`, `task.md` (the plan), `docs/handoff/16-file-links-report.md` (file fetch: guard scope, deny list, `human_only` files, Range), `docs/handoff/18-sandbox-worktree-network-report.md` (hardened host-side git: `gitExec`, `--attr-source=<empty tree>`), `internal/guard/filefetch.go` (`CheckFileFetch`, `isSecretName`, `openSnapshot`), `internal/guard/filelinks.go`, `internal/modules/files/` (fetch requests, `FileLinks`, `NormalizePath`), `internal/gateway/gateway.go` (connection registry, `ReportActivity`: how an ephemeral frame reaches chosen connections), `internal/guard/ws.go` (the guard's WebSocket frames), `internal/httpserver/web/index.html` and `web/markdown.js` (viewer, chips, I18N), `internal/modules/hosts/` (assignments: root dir, worktree, grants).

Do the parts in this order; one commit (or a few) per part; after each part `gofmt`, `make test`, `make lint`, `make test-web`. Run `go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/`. If a store or migration changes, `make test-pg` too (uncached, `GOFLAGS=-count=1`; if `make dev` cannot build the server image on this machine, start only the Postgres container: `docker compose -f deploy/compose.dev.yaml up -d postgres`, and stop it at the end). Contract first for every REST/WebSocket change (`api/openapi.yaml`, `make gen`).

## Decisions (Norbi, 2026-10-26; do not change them)

- **Whose folder:** in a DM, the DM's agent. In a channel, an **agent picker** at the top of the panel lists the channel's agent members; the page remembers the last choice per channel (localStorage is fine). **You see exactly what that agent sees**: a full-access agent's root folder (or its worktree), a sandboxed agent's granted folders and worktree. No other rule, no new folder setting.
- **Freshness:** every expand / open / search asks the guard again. No cache in this round (a later round may add one).
- **File name search** and **git status** are in this round.
- **Viewer: Monaco editor** (`monaco-editor`, MIT), read-only. Note for the report: VS Code extensions do not run in Monaco (they need VS Code's extension host); this round uses Monaco's built-in language support (Monarch tokenizers, ~80 languages), find, minimap, go-to-line. Markdown files also get the formatted view from `web/markdown.js` (toggle).
- **Humans only, read-only.** Agents never use these endpoints (agent token → 403). Nothing is written to the agent's folder.

## 1. Ephemeral folder requests through the server (no database)

Listing and searching must answer in about a second, so they do **not** use the round-16 request table. The server relays them in memory:

- `POST /api/v1/agents/{agentId}/fs` (humans only) with `{channel_id, op, path?, query?}`; `op` is `list` | `search` | `git_status`.
  - Allowed only if the caller can read `channel_id` **and** the agent is a member of that channel (a DM counts). Otherwise 403. Another workspace's agent → 404. The agent must have a host assignment.
  - The server keeps the request in memory (id, agent, op, args, a channel to wait on), pushes an **ephemeral WebSocket frame** `fs.request {request_id}` to that agent's open connections only (the gateway's registry), and waits up to **15 s** for the answer; then 504 with a plain message ("a gép nem válaszol").
  - Not stored, not in the event log; but each request is audited once (who, which agent, op, path — never file names from the answer, never content).
- Guard side (agent token, the agent of the request only): `GET /api/v1/agent/fs/{requestId}` returns the op and args; `POST /api/v1/agent/fs/{requestId}/result` delivers the JSON answer (max 2 MB) or `{error_code}`. A request for another agent → 404. An answer after the timeout → 410.
- **Opening a file reuses round 16** (`POST /messages/{id}/file-fetch` needs a message; add a sibling `POST /api/v1/agents/{agentId}/file-fetch` `{channel_id, path}` with the same channel rule as above, the same guard path, the same `human_only` stored file, Range and limits). The "path occurs in the message" rule of round 16 stays for the message route; the new route is bounded by the guard scope instead.
- Tests: e2e over the contract (human with access → answer from a fake guard; human without channel access → 403; agent token → 403; agent not in the channel → 403; timeout → 504; late answer → 410; other agent's request id → 404); gateway: the frame goes only to that agent's connections.

## 2. Guard: list, search, git status

`internal/guard/fsbrowse.go`, reusing the scope logic of `CheckFileFetch` (refactor it into one function that answers "may this agent's human see this real path"; do not copy it):

- **Scope roots:** the agent's root folder or worktree (full access), or its active grants and worktree (sandboxed). The tree's top level is these roots (several roots for a sandboxed agent).
- `list(path)`: one level, entries `{name, kind: dir|file|symlink, size, mtime}`; sorted dirs first, then names (case-insensitive). **Hidden from the listing:** the deny list (`deniedPaths`, `isSecretName` names, the exchange dir, the guard's config/token files), `.git`. **Collapsed** (listed as a dir with `heavy: true`, children only on explicit expand): `node_modules`, `vendor`, `.venv`, `venv`, `__pycache__`, `dist`, `build`, `target`, `.next`. At most 500 entries per directory, then `truncated: true`. Symlinks: shown, resolved, and refused (not listed further) if they leave the scope.
- `search(query)`: case-insensitive substring (and `*` glob) on file names under the scope roots; skips hidden and heavy dirs; stops at 200 results or 3 s, says which; returns relative paths with their root.
- `git_status()`: for each scope root that is (or is inside) a git work tree: branch, `ahead/behind` if there is an upstream, and the changed files (`status --porcelain=v1 -z`, parsed from NUL-separated records, not text lines). Use the **hardened host-side git** from round 18 (`gitExec` with `core.hooksPath=/dev/null`, `core.fsmonitor=false`, `GIT_CONFIG_NOSYSTEM`, `GIT_CONFIG_GLOBAL=/dev/null`, and `--attr-source=<empty tree>` so no filter or textconv runs), 5 s timeout. A repo that is too big or slow → `error_code: timeout`, the tree still works.
- All paths in answers are relative to their root; real paths never leave the guard except the root folders themselves. Every op is logged (agent, op, path; no names from results, no content).
- Tests: scope for full-access and sandboxed agents; deny list hidden at every level; `.git` hidden; heavy dirs collapsed; 500-entry cap; symlink out of scope refused; search limits; git status on a repo with modified/added/untracked/deleted files and a file name with spaces and a newline (the `-z` parsing); a `.gitattributes` filter in the repo does **not** run during `git_status` (same test idea as round 18); timeout handling.

## 3. Dev page: the panel

- A **"📁 Fájlok"** button next to the message box (DM and channel). It opens a right-side panel (full screen on phones, like the agent panel of round 15). In a channel: the agent picker on top (agent members only; none → a hint).
- **Tree:** lazy (expand loads one level), git badges next to names (M, A, D, ?, R with colours; a dot on folders that contain changes), the branch name and ahead/behind on each repo root, a refresh button, a search box (results as a flat list; click opens the file).
- **Viewer: Monaco**, vendored, not from a CDN:
  - Put `monaco-editor@0.52.2`'s `min/vs` folder (no source maps, no `esm/`, no `dev/`) under `internal/httpserver/web/vendor/monaco/vs/`, with its `LICENSE` and a short `README` (version, source, how to update). Get it with `npm pack monaco-editor@0.52.2` into a temp folder; do not add a `package.json` to the repo. Report the added size.
  - Load the AMD loader **only when the panel first opens**; workers from the same origin.
  - Read-only, line numbers, language from the file extension (a small mapping table; unknown → plaintext), the theme follows the page (light/dark), `wordWrap` toggle.
  - `.md`: a "Formázott / Nyers" toggle; formatted = `ColonyMarkdown.render`. Images: preview. Big files: the round-16 head/tail Range view (Monaco gets the shown part, with a note "1–2000. sor a 48 000-ből").
  - The file content comes from the new agent file-fetch route (part 1); show "állapot: 10:42" and a refresh button as the round-16 viewer does.
- **References:**
  - Selecting lines in Monaco (or clicking a line number) shows "Hivatkozás beszúrása": inserts into the message box `path:42` or `path:42-58` (path relative to the agent's root). In a channel the reference is prefixed with the agent: `@legy-claude:src/x.go:42-58`. A file in the tree has the same button (path only).
  - In messages, references render as chips (`📄 server.go:42-58`); clicking opens the panel at that agent, file and lines (Monaco `revealLinesInCenter` + selection). Extend the link recognition in `web/markdown.js` and the guard/server `FileLinks` so `path:42` / `path:42-58` suffixes are understood (the path part still has to pass every existing rule; the line part is only for display). Relative references are resolved against the agent's root: in a DM the DM's agent, in a channel the `@handle:` prefix; a relative reference without a resolvable agent stays plain text.
  - The guard's direct and work prompts get one line: "A reference like `path:42-58` in a message means those lines of that file, relative to your root folder."
- All strings via I18N (hu, en). Check desktop and phone (390 px) with Playwright-style screenshots or describe them precisely.
- `make test-web`: tests for the new reference parsing (`a/b.go:12`, `a/b.go:12-30`, `@h:a/b.go:7`, a Windows-looking path, `http://x/y:8080` must not become a reference, a time `10:42` must not become one).

## Out of scope (say so in the report)

Editing files, a cache, a full-text (content) search, VS Code extensions, opening files of agents in channels you cannot read.

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (the relay, the endpoints, the WebSocket frame, the guard ops and their limits, the reference format), `docs/01-terv.md` (a short section, implemented), `task.md` (move the plan item to "Kész" with the date and what is left for later: cache, content search; keep the commit ids for the reviewer), `CLAUDE.md` schedule. Then the report `docs/handoff/20-file-browser-report.md` (Hungarian): commits, decisions where this prompt was open, the size Monaco added, what is left out, the outputs of `make test`, `make lint`, `make test-web`, `make gen-check`, the `-race` run (and `make test-pg` if run), how the panel looks at desktop and phone width, and what the reviewer should look at first.
