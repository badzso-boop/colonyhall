# Handoff prompt 06: the team wiki (for Antigravity CLI)

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/06-wiki-prompt.md)"
```

---

You are continuing the Go project **Colonyhall** in the current directory. Claude Code designed it and reviews your work afterwards. The core rule of this task is an authorization rule — enforce it on the server and test it from every side.

## Get the rhythm (read before coding)

1. `CLAUDE.md` (binding rules, commands, schedule).
2. `docs/handoff/01-direct-mode-prompt.md` section 2 **"Ground rules"** — all apply (new branch from `main`, no commits to `main`, no push, **no real `claude`/`agy` calls**, contract-first, `make gen`, storetest memory+Postgres, e2e contract tests, per-step checks, `make test-pg` + `make gen-check` at the end, one commit per step, no AI attribution).
3. `docs/01-terv.md`: section **"Közös tudás és döntésnapló"**, the part **"Döntés (2026-10-16): wiki a szerveren"** is your specification.
4. Code: any module as a template (`internal/modules/schedule/` is recent and small), `internal/modules/conversation/mcptools/tools.go` (MCP tools; actor re-checked per call; tool output is compact text), `internal/guard/direct.go` (`DirectPrompt`), `internal/httpserver/web/index.html` (dev page panels; `textContent` only).

Branch: `git switch -c feature/wiki`.

## Task

### Server (new module `wiki`)

- Migrations: `wiki_page` (`id`, `workspace_id`, `slug` unique per workspace, `title`, `tags` text[], `created_by`, `created_at`, `deleted_at` nullable) and append-only `wiki_revision` (`page_id`, `rev` int, `body`, `kind` (`create` | `append` | `edit` | `delete` | `restore`), `author_id`, `created_at`, primary key `(page_id, rev)`). The current body is the latest revision's body. Never UPDATE a revision, never DELETE anything.
- Slug from the title: lower case, accents folded (á→a, ő→o, ű→u …), non-alphanumerics → `-`, max 80; collision → `-2`, `-3`.
- Rules (enforce in the service, test them all):
  - Any actor of the workspace may read the index and pages.
  - **Agents** may create a page and **append** to an existing page. Append = the server adds a new section at the end: `\n\n---\n*@handle, 2026-10-16 09:30:*\n\n<text>`. Agents can never replace or remove existing text, rename, retag or delete a page.
  - **Humans** may also edit (replace the whole body; record `edit`), rename/retag, delete (soft: `deleted_at`, revision `delete`) and restore.
  - Limits: title ≤ 120, body ≤ 50 000 characters (append counts against it), append text ≤ 8 000; reject empty text.
  - Halted actors cannot write.
- REST: `GET /api/v1/wiki?q=` (index: slug, title, tags, first line of the body ≤ 120 chars, updated_at, deleted pages hidden except for humans with `?deleted=1`), `GET /api/v1/wiki/{slug}` (current body + rev), `GET /api/v1/wiki/{slug}/revisions`, `POST /api/v1/wiki` (create), `POST /api/v1/wiki/{slug}/append`, `PUT /api/v1/wiki/{slug}` (human edit, needs `base_rev` and answers 409 if someone changed it meanwhile), `DELETE /api/v1/wiki/{slug}` (human), `POST /api/v1/wiki/{slug}/restore` (human). Events `wiki.created` / `wiki.appended` / `wiki.edited` / `wiki.deleted` / `wiki.restored` (no body in the payload) for the audit log.
- MCP tools for agents: `wiki_index` (query? → one line per page: slug, title, first line; cheap), `wiki_read` (slug → body, truncated at 8 000 with a note), `wiki_create` (title, body, tags?), `wiki_append` (slug, text). Tool descriptions short.
- OpenAPI first, `make gen`, storetest (memory + Postgres), e2e contract tests.

### Guard

In `DirectPrompt` add one sentence: before working, check the team wiki (`wiki_index`) for a page about the task and follow it; record durable know-how with `wiki_append` (never secrets).

### Dev page

A "Wiki" button: index with search; page view (render as plain text with line breaks — no HTML from content); for humans an editor (textarea, saves with `base_rev`), delete/restore; for everyone an "append" box.

## Tests (no real CLIs)

- e2e for every rule: agent create ok; agent append adds the signed section at the end; agent edit/delete/restore/rename → 403; human edit with stale `base_rev` → 409; delete hides from the index, restore brings it back; revisions keep every version; limits → 400; halted → 403; other workspace → 404.
- MCP: an agent session lists, reads, creates and appends through the tools.
- storetest for the store, slug collisions, accents.
- `-race` on e2e.

## Docs

`docs/02-muszaki-specifikacio.md`: "Wiki (megvalósítva)" paragraph (tables, rules, endpoints, MCP tools); `docs/01-terv.md` MCP tool list stays as is; `CLAUDE.md` schedule. Hungarian.

## Report

Write `docs/handoff/06-wiki-report.md` (Hungarian) as before, commit, print a 5-line summary, stop. Do not merge.
