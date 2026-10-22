# Handoff prompt 17: markdown in chat messages (one safe renderer for the chat and the file viewer)

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/chat-markdown ../new-chat-worktrees/chat-markdown main
cd ../new-chat-worktrees/chat-markdown
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/17-chat-markdown-prompt.md)"
```

---

Branch: `feature/chat-markdown`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/chat-markdown` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/chat-markdown`; never merge, never push.

## Why

Agents answer in markdown (bold, lists, code, tables, links); the chat shows the raw text (`**PARTIAL**`, `| a | b |`, `[x](file:///…)`). The file viewer (round 16) already has a small safe markdown renderer (`buildSafeMarkdownDOM`, `renderInlineMarkdown` in `internal/httpserver/web/index.html`), with tables since `51a065a`. This round makes **one** renderer, moves it into its own file with tests, and uses it for chat messages too.

This is a front-end round: **no API, database or guard change.** If you think one is needed, stop and write it in the report instead.

Read first: `CLAUDE.md`, `internal/httpserver/web/index.html` (`renderMsg`, `renderAgentBodyWithFileChips`, `parseFileLinkSegments`, `buildSafeMarkdownDOM`, `renderInlineMarkdown`, the error/approval tokens `[error:<id>]` / `[approval:<id>]`, the I18N blocks), `internal/httpserver/httpserver.go` (the `web/` folder is embedded and served as static files), `docs/handoff/16-file-links-report.md`.

## Decisions (Norbi, 2026-10-22; do not change them)

- **Every message is rendered as markdown**, from humans and agents alike (like Slack).
- **Safety:** the renderer builds DOM nodes (`createElement`, `textContent`); **never** `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`, or setting `on*` attributes from content. Raw HTML in a message is shown as text. Links only for `http:` and `https:` (with `target="_blank" rel="noopener noreferrer"`); anything else (`javascript:`, `data:`, `vbscript:`, `file:` as a link target, relative URLs) is plain text. **No images from URLs** (`![x](url)` is shown as a link if http(s), else as text): loading remote images would leak who reads what.
- **Local file links stay chips:** `file:///…` and the local paths that round 16 recognises become the existing "📄 name" chips (also when they are the target of a markdown link: `[scheduler.log](file:///…/scheduler.log)` → chip "📄 scheduler.log"), only in **agent** messages, exactly as today (the server only serves paths that occur in an agent's message). In human messages they stay text.
- **`snake_case` and identifiers are not italic:** `_` and `*` emphasis only at word boundaries (CommonMark's left/right-flanking rule is enough in its simple form): `PROFIT_TARGET`, `run_id=9`, `file_name.md`, `2*3*4` stay as they are. Norbi's trading reports are full of these.

## 1. One renderer in its own file: `internal/httpserver/web/markdown.js`

- A classic script (no build step, no modules needed by the page), loaded by `index.html` with `<script src="markdown.js"></script>` before the main script; it defines one global object, e.g. `window.ColonyMarkdown`.
- **Two layers**, so the logic is testable without a browser:
  1. `parse(text) → blocks` — pure function, no DOM: a small tree of plain objects (`{type: "paragraph", children: [...]}`, `{type: "text", text}`, `{type: "link", href, children}`, `{type: "filelink", path, name}`, …).
  2. `render(blocks, options) → DocumentFragment` — the only part that touches the DOM. `options`: `{ fileChips: boolean, onFileClick(path), onMention(handle), knownHandles: Set }`.
- Supported (and nothing else):
  - blocks: paragraphs (single newlines inside a paragraph are kept as line breaks — chat users expect that), headings `#`…`######` (in chat shown as bold text, a little larger for `#`/`##`; in the viewer as real headings), fenced code blocks ` ``` ` (with an optional language label, no highlighting, a "másolás" copy button), blockquote `>`, unordered and ordered lists with **one level of nesting** (two or more spaces of indent), task items `- [ ]` / `- [x]` (shown as ☐ / ☑, not clickable), tables (GitHub style, as in `51a065a`, horizontally scrollable on phones), horizontal rule `---`/`***`/`___`;
  - inline: `**bold**`, `__bold__`, `*italic*`, `_italic_` (word-boundary rule above), `~~strike~~`, `` `code` `` (nothing inside is parsed), links `[text](url)`, bare `http(s)://` URLs auto-linked (trailing `.,;:!?)` not part of the URL), backslash escapes (`\*` → `*`), `@handle` mentions highlighted (only handles in `knownHandles`; clicking opens the actor panel through `onMention`).
- Limits: inputs over 20 000 characters are rendered as plain text (the server's message limit); nesting depth is bounded; the parser is linear-ish (no regex with catastrophic backtracking on long lines — test a 20 000-character line of `*` and `_`).
- Replace `buildSafeMarkdownDOM` / `renderInlineMarkdown` / the chip parser in `index.html` with this file; the file viewer uses `render(parse(text), { fileChips: false })` with real headings.

## 2. Chat messages

- `renderMsg` renders the body through the renderer. Keep, in this order and unchanged in behaviour: the `[error:<id>]` button, the `[approval:<id>]` inline approval, attachments, Gitea link cards, reactions, the reply/thread line. Remove the error/approval tokens before parsing, as today.
- **Long messages collapse:** a body taller than about 20 lines (measure the rendered height, e.g. `max-height` + a check of `scrollHeight`) shows a fade and a "Több" / "Kevesebb" toggle. Agents' reports can be 8000 characters.
- The thread line ("↪ in thread: …") and every other short preview (sidebar, notifications, the reply bar) show **plain text**: add `ColonyMarkdown.plain(text)` that strips the markup (`**x**` → `x`, `[t](u)` → `t`, code fences removed, table pipes → spaces) and use it there.
- Styles: compact, chat-sized (paragraph margins small, code blocks scroll horizontally, tables scroll inside the message, quotes with a left border); same tokens/variables as the rest of the page; check desktop and phone (390 px).
- All new UI strings (copy button, Több/Kevesebb, copied!) in the I18N blocks, hu and en.

## 3. Tests

- `internal/httpserver/web/markdown.test.mjs`, run with Node's built-in test runner (`node --test`), testing **`parse`** (pure): every supported construct; the word-boundary rule (`PROFIT_TARGET`, `run_id=9`, `a_b_c`, `2*3*4`, `**PARTIAL**`); links (`javascript:alert(1)`, `JaVaScRiPt:`, `data:text/html,…`, ` javascript:` with leading space, `http://ok`) → only http(s) become links; raw `<script>` / `<img onerror>` stay text nodes; `file://` and local paths become `filelink` nodes (also inside `[name](file:///…)`), and plain `https://…/home/x` does not; a real agent answer from Norbi's chat (the `run_id=9` report with two file links); tables; nested lists; code blocks keep their content verbatim; the 20 000-character pathological line finishes fast.
- A Go test `internal/httpserver/markdown_test.go` that runs `node --test web/markdown.test.mjs` when `node` is on the PATH and **skips** when it is not, so `make test` covers it on this machine. Add `make test-web` as a shortcut.
- A DOM-level check is optional; if you add one, use only what Node provides (no new npm dependencies, no `package.json`).

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (the renderer, what is supported, the safety rules), `task.md` (move "Markdown megjelenítése a felületen" from "Tervezendő" to "Kész" with the date; keep the commit ids for the reviewer), `CLAUDE.md` schedule. Then the report `docs/handoff/17-chat-markdown-report.md` (Hungarian): commits, decisions where this prompt was open, what is left out, the outputs of `make test`, `make lint`, `make test-web`, a description of the chat at desktop and phone width (a long agent report, a table, a code block, a human message with `snake_case`), and what the reviewer should look at first. No `make test-pg` is needed (no store change); say so.
