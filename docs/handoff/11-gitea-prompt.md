# Handoff prompt 11: git links and Gitea integration

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/11-gitea-prompt.md)"
```

---

Branch: `feature/gitea`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 5, and "Fájlok és git linkek" (git part). The CLAUDE.md "Bővítés = új adapter" rule applies: a `GitProvider` port with a Gitea adapter; GitHub/GitLab later as further adapters.

## Server (new module `git`)

- Config: `COLONY_GITEA_URL`, `COLONY_GITEA_TOKEN` (read-only token is enough for unfurling), `COLONY_GITEA_WEBHOOK_SECRET`. Without them the module is off (no errors, no unfurls).
- **Link cards:** when a message contains a URL on the configured Gitea host pointing to a repo, PR, issue or commit, the server fetches the title/state/author via the Gitea API **asynchronously** (an event consumer, not in the send path), caches it (table `git_link`: url, kind, title, state, fetched_at; refresh after 10 min), and attaches it to the message API response (`link_cards`). Only URLs on the configured host are fetched (no SSRF to arbitrary hosts); timeouts; size limits.
- **Webhook:** `POST /api/v1/hooks/gitea` verifies the HMAC signature (`X-Gitea-Signature`, constant-time compare) and maps events (pull_request opened/closed/merged, push, status/CI) to a short message in a channel chosen per repository (table `git_repo_channel`: repo full name → channel; admins manage it via REST + a small dev page form). The message is posted by a system/bot actor created on first use (`@gitea`, kind agent, no token, cannot be woken). Unknown repos are ignored. Replays are idempotent (delivery id).
- MCP for agents: `git_status` (PR/issue URL or `owner/repo#12` → one line: title, state, checks).
- Contract-first, storetest, e2e with an `httptest` fake Gitea (API + signed webhook): card fetched, cached, wrong host ignored, bad signature 401, mapping, idempotency.

## Dev page

Link cards under messages; a "Gitea" section on the "Gépek" panel (admins) for repo → channel mappings.

## Docs

`docs/02-muszaki-specifikacio.md` (GitProvider, endpoints, env vars), plan section marked, `CLAUDE.md` (env vars, schedule). Hungarian.
