# Handoff prompt 10: file sharing in the chat

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/10-files-prompt.md)"
```

---

Branch: `feature/files`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 4, and the section "Fájlok és git linkek" (file part only; git links are task 11). Read `internal/modules/conversation/` (messages), `deploy/compose.dev.yaml` (add a volume for the files), `internal/platform/config`.

## Server (new module `files`)

- Port `StorageBackend` (Put, Open, Delete-never: files are kept; Stat) with a filesystem adapter (`COLONY_FILES_DIR`, default `./data/files` locally, a named volume in compose). Store files by content hash (sha256) under the root; never use the client's filename as a path.
- Table `file` (append-only): `id`, `workspace_id`, `uploaded_by`, `channel_id`, `name` (display only, sanitized, ≤ 200), `mime` (sniffed from the content, not trusted from the client), `size`, `sha256`, `created_at`. A message references files via a new `message_file` link table (or a `file_ids` column) — choose and document; the message API returns attachments (id, name, mime, size).
- `POST /api/v1/channels/{id}/files` (multipart; size limit `COLONY_MAX_UPLOAD_MB`, default 25), `GET /api/v1/files/{id}` (metadata), `GET /api/v1/files/{id}/content` (download with `Content-Disposition: attachment`, `X-Content-Type-Options: nosniff`, never inline HTML/SVG), access only for actors who can read the file's channel. Sending a message can attach uploaded file ids (only files of the same channel, uploaded by the sender).
- MCP for agents: `upload` (a local path relative to the agent's root dir is **not** something the server can read — so the tool takes base64 content + name; the guard is not involved) and `download` (file id → content as text if text-like and small, otherwise a short description and the download URL). Size limits apply; document that agents with full access can also just use the URL.
- Contract-first, storetest (memory adapter for storage too), e2e: upload, attach, download, access denied from another channel, MIME sniffing, limits, path traversal attempts in names.

## Dev page

Paperclip button next to the message box (upload, then send with the attachment), attachments under messages (image thumbnails via the content endpoint for images only, otherwise a download link).

## Docs

`docs/02-muszaki-specifikacio.md` (Fájlok paragraph), plan section marked, `CLAUDE.md` schedule and env vars. Hungarian.
