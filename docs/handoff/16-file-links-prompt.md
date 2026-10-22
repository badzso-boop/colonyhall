# Handoff prompt 16: open agents' local file links in the UI (fetched by the guard, read-only, humans only)

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/file-links ../new-chat-worktrees/file-links main
cd ../new-chat-worktrees/file-links
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/16-file-links-prompt.md)"
```

---

Branch: `feature/file-links`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/file-links` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/file-links`; never merge, never push.

## Why

Agents link files on their machine in their answers (`file:///home/szmajli01/homelab/trader/logs/scheduler.log`, or a bare `/home/…/report.md`). In the browser such a link opens nothing: the file is on the homelab or the PC. Norbi's idea (decision, 2026-10-22): **the agent only links; the guard fetches the file when a human clicks, the server stores it, the UI shows it read-only.** The agent does not paste file contents into the chat, which saves its context and tokens, and the display is deterministic code, not the model.

A second, related bug: the guard clips the final answer (`reply`: 600 characters, `replyLong`: 8000, `clip` in `internal/guard/prompt.go`), so **a link at the end of a long answer can be cut off**. Part 1 fixes that.

Read first: `CLAUDE.md`, `task.md` ("Tervezendő" → "Fájlok az agentek üzeneteiben"), `docs/01-terv.md` (permissions, "Gépek, mappahozzáférés és sandbox"), `internal/guard/guard.go` (`reply`, `replyLong`, `work`), `internal/guard/direct.go`, `internal/guard/paths.go` (`CheckGrant`, `deniedHome`), `internal/guard/host.go` (`policy`: grants, roots), `internal/guard/client.go`, `internal/modules/files/` (upload, storage, attachments, content endpoint), `internal/gateway/gateway.go` (event visibility), `internal/httpserver/web/index.html` (dev page: `el()`, `textContent`, I18N blocks, `showPanel`, the Machines panel).

The production server runs on the homelab and the dev stack on this machine is normally stopped. For `make test-pg` start it from the worktree (`make dev`) and **stop it at the end (`make down`)**. Run `make test-pg` really (with `GOFLAGS=-count=1`), not from cache, and report its output.

Migrations: the next free numbers (`main` ends at `00024`).

Do the parts in this order; one commit (or a few) per part; after each part `gofmt`, `make test`, `make lint`.

## Decisions (Norbi, 2026-10-22; do not change them)

- **Only humans may request a file.** Agents never: otherwise a sandboxed agent could read a full-access agent's files on another machine through a link. Agent tokens get 403 on every endpoint of this feature.
- **The fetched content is for humans only.** It never goes back to an agent: not in the inbox, not in attachments listed to agents, not through `/files/{id}/content` with an agent token (403), not through MCP tools.
- **Limits are generous and configurable** (everything runs on the local network / Tailscale): default **200 MB for images, 100 MB for other files**; admins change them in the UI. The server sends the current limits with each request to the guard; the guard enforces them too.
- **No new line in the shared agent rules:** agents already link files.
- **Lost links (part 1):** the guard sends the links that the clipping removed in a separate follow-up message. It is posted **by the same agent** (the guard has only that agent's token; a privileged "system" identity the guard could post as would be a new, broader credential, so not in this round), clearly marked, e.g. "📎 Fájlok:" + one path per line.

## 1. Guard: never lose a file link when clipping (bug)

- A small pure function in the guard: `FileLinks(text string) []string` finds `file://` URLs (also inside markdown links `[x](file:///…)`) and absolute local paths that look like files (`/home/…`, `/mnt/…`, `/root/…`, `/srv/…`, `/tmp/…`, `/var/…`, `/opt/…`, `~/…`), stopping at whitespace, quotes, backticks, `)`, `]`, `>`, and trailing `.,;:!?`. Decode `file://` URL escapes. Keep order, drop duplicates. Unit tests with real-looking agent answers (Hungarian text, markdown, code spans, links at the very end, two links on one line, a URL that is **not** a local path like `https://…/home/x` must not match).
- In `reply` and `replyLong` (and anywhere else the guard clips an agent's final text): compute the links of the **full** text before clipping; after clipping, every link that is not present **in full** in the clipped text is sent in one follow-up message in the same thread: a neutral `📎` line, then the paths, one per line (the answer's language is not known to the guard), `elaborate` if it exceeds the short limit, with its own `client_msg_id` (`<id>-files`) so a retry does not duplicate it. A follow-up failure is logged and does not fail the run.
- Tests (fake adapter): a 9000-character answer ending with a path → the clipped reply plus a follow-up with exactly that path; a short answer → no follow-up; a path cut in half by the clip counts as lost.

## 2. Server: file fetch requests

Model (module `internal/modules/files`, or a new `filefetch` module if it fits the layering better; decide and say why in the report):

- Table `file_fetch_request`: `id`, `workspace_id`, `message_id`, `channel_id`, `agent_id` (the author of the message), `requested_by` (human), `path` (as written in the message), `state` (`pending` | `done` | `failed`), `file_id` (nullable), `error_code` (nullable: `host_offline`, `not_found`, `denied`, `too_large`, `not_a_file`, `timeout`), `created_at`, `completed_at`. Rows are never deleted.
- Table (or settings row) for the limits per workspace: `max_image_mb` (default 200), `max_file_mb` (default 100). Validate 1…2048.
- `POST /api/v1/messages/{messageId}/file-fetch` `{path}` — **humans only**. Checks, in order: the caller can read the message's channel (`CanRead`); the message's author is an **agent** of the same workspace; **the requested path occurs in that message's body** (compare after the same normalisation as `FileLinks`: decode `file://`) — so nobody can ask for an arbitrary path; the agent has a host assignment. If a `done` request for the same `(message_id, path)` exists and `refresh` is not set, return it (200) instead of a new one. Else create a `pending` request (201) and append event `file.fetch_requested` (channel-scoped, payload: request id, agent id, message id — **no file content**). Audit it.
- `GET /api/v1/file-fetch/{id}` — the request with state, error code, and the file (`FileAttachment` + `fetched_at`) when done. Humans who can read the channel.
- Guard side, **agent token** of the message's author agent (the guard calls it for its own agents; any other agent → 404):
  - `GET /api/v1/agent/file-fetch?state=pending` — this agent's pending requests, each with `path` and the current limits (bytes).
  - `PUT /api/v1/agent/file-fetch/{id}/content` — streamed body (do not read it all into memory; enforce the limit on the stream; `Content-Type`, `X-File-Name`, `X-File-Mtime`), stored through the existing file storage, marked **human-only** (new column on the file, e.g. `human_only boolean not null default false`); the request becomes `done`. Event `file.fetch_completed`.
  - `POST /api/v1/agent/file-fetch/{id}/fail` `{error_code}` → `failed`, event `file.fetch_failed`.
- A request still `pending` after 2 minutes becomes `failed` with `host_offline` (a sweep on the server's Clock; test with the manual clock).
- **Human-only files:** `/files/{id}`, `/files/{id}/content` and every list that shows attachments return 403/omit them for agents. Humans need `CanRead` on the channel as for any file.
- `GET /api/v1/settings/file-fetch` (humans) and `PUT` (admins only; event + audit).
- The upload limit for this path is the fetch limit, not `COLONY_MAX_UPLOAD_MB`; check that the HTTP server and the compose setup do not cap the body lower (and if `http.MaxBytesReader` is used, set it from the limit).
- Contract first: `api/openapi.yaml`, `make gen`. storetest (memory + Postgres) for the new store(s); e2e over the contract.

## 3. Guard: serve the requests

- The guard already watches `/ws/ui`; on `file.fetch_requested` for one of its agents (and on every fallback poll) it calls `GET /api/v1/agent/file-fetch?state=pending` for that agent and handles each request. Requests run concurrently with runs but at most 2 at a time per guard; never block an agent's run.
- **Scope = what the linking agent may see, never more:**
  - sandboxed agent: only inside its **active folder grants** (`CheckGrant`, the same re-check as `policy()`; read or write grants both allow reading);
  - full-access agent: inside the guard's `roots` (`guard.yaml`).
  - Always denied: `deniedHome`, the guard's own config and token files (`~/.config/colonyhall`, `host_token_file`, the Claude token file), the `exchange_dir`, and names that hold secrets: `.env`, `.env.*`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`, `*.p12`, `credentials*`, `*.token`, `.netrc`, `.git-credentials`. → `denied`.
  - `~/` expands to the guard user's home. Resolve symlinks (`filepath.EvalSymlinks`) and re-check the **real** path against the scope and the deny list. Only regular files (`not_a_file` for directories, devices, sockets, fifos). Missing → `not_found`.
  - Size: image MIME (by extension and sniffing the first 512 bytes) → image limit, everything else → file limit; larger → `too_large` (do not send a partial file).
- Stream the file to `PUT …/content`; never log its content; log path, size, result.
- Tests: scope for sandboxed and full-access agents, every deny rule, a symlink pointing out of the scope, a directory, a too-large file (use a small limit in the test), a server error does not crash the guard, a request for another guard's agent is ignored.

## 4. Dev page: open the file

- In **agent** messages, every match of the same link rules (port `FileLinks` to JS with the same test strings, kept small) becomes a clickable "📄 <file name>" chip; the rest of the text stays as it is (`textContent`, no `innerHTML`).
- Click → `POST …/file-fetch` → a viewer dialog (full-screen on phones) shows "betöltés…", then the result through the `file.fetch_completed` / `file.fetch_failed` frames (or polling `GET /file-fetch/{id}` every 2 s as a fallback):
  - text, log, JSON, YAML, code: monospace, read-only; files over 2 MB show the **first** and **last** 1 MB with a "megjelenítés: eleje / vége" switch (logs are read from the end), using HTTP `Range` on the content endpoint (add `Range` support if missing);
  - markdown (`.md`): rendered safely — build DOM nodes, never `innerHTML`; headings, bold, italic, inline code, code blocks, lists, links (only `http`/`https`, `rel="noopener noreferrer"`, `target="_blank"`), everything else as plain text; a "nyers" toggle;
  - images: `<img>` from the content URL;
  - anything else: name, size, MIME and a download button.
  - The header shows the path, the agent, and "állapot: 10:42" (`fetched_at`); a "frissítés" button asks again with `refresh`.
  - Errors in plain words (hu/en): the machine is offline, the file does not exist, sharing this file is not allowed, too large (with the limit), not a file.
- Admins: in the Machines panel a small "Fájlmegnyitás" box with the two limits (MB) and save.
- All strings via the I18N blocks (hu and en), no hardcoded text. Check desktop and phone width (390 px).

## Security tests (must exist; each fails without its check)

- An agent token on every new endpoint → 403 (or 404 for another agent's requests).
- A human who cannot read the channel → 403 on `POST …/file-fetch` and on `GET /file-fetch/{id}`.
- A path that is not in the message → 400; a message written by a human → 400.
- A human-only file is not readable with an agent token and does not appear in an agent's inbox or attachment lists.
- Guard: `../` tricks, a symlink out of the scope, `.env` inside a granted folder, the exchange dir → `denied`.

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (endpoints, events, the human-only flag, the guard's scope rules, limits), `docs/01-terv.md` (mark it implemented), `task.md` (move "Fájlok az agentek üzeneteiben" to "Kész" with the date; add the clipped-link bug as done; keep the commit ids for the reviewer), `CLAUDE.md` schedule. Then the report `docs/handoff/16-file-links-report.md` (Hungarian): commits, decisions where this prompt was open (module placement, MIME detection), what is left out, the outputs of `make test`, `make lint`, `make test-pg` (uncached), `make gen-check`, `go test -race ./internal/guard/ ./internal/e2e/`, a description of the viewer at desktop and phone width, and what the reviewer should look at first.
