# Handoff prompt 40: security hardening

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/security ../new-chat-worktrees/security main
cd ../new-chat-worktrees/security
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/40-security-prompt.md)"
```

---

Branch: `feature/security`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/security`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. Next free migration: check `migrations/`.

After each part: `gofmt`, `make test`, `make lint`; `make test-pg` when a store changed; `make test-web` for PWA parts. At the end `go test -race ./internal/...`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it.

**Do not touch the homelab or any shared infrastructure.** Do not run attacks against the live server (`100.85.111.88`); test against the local dev stack or the memory server.

## Why (Norbi, 2026-11-05)

Claude Code assessed the application's security (2026-11-05) and found the foundations strong: argon2id passwords, hashed tokens, `HttpOnly`/`SameSite` session cookie, CSRF tokens, sqlc (no SQL injection), React without raw HTML, uploads served as attachments with `nosniff`, per-call authorization, a hash-chained audit log, SSRF protection in web-read, a non-root container, the server bound to the Tailscale address only.

**Already fixed** before this round (branch `fix/security-quick`): security headers (a strict CSP for the PWA with the inline theme script allowed by hash, `X-Frame-Options`, `Referrer-Policy`, `Permissions-Policy`, COOP), per-request read and write deadlines (WebSockets and `/mcp` excepted), `IdleTimeout`, `MaxHeaderBytes`, and the dependency updates (`golang.org/x/net` 0.62.0, Go 1.27.3; `govulncheck`: no vulnerabilities).

## Norbi's decisions (binding)

- **Not in this round:**
  - HTTPS (the server is LAN/Tailscale-only during development; later);
  - a non-root user for the PC guard and its agents;
  - removing the homelab agent with `bypassPermissions` (both are kept for testing);
  - the homelab's Docker ports on the LAN (they are needed).
- **In:** a durable login limit **with an allowlist**, the dependency hygiene, a full code review against the standards with fixes, and container hardening **only if Norbi approves it in the chat** (part 3).

## 1. Durable login and release limits with an allowlist

Today the limiter is in memory (lost on restart) and keyed by handle only.

- **Store the failures in the database** (migration; a store with memory and Postgres adapters and `storetest`): per handle **and** per client IP, a sliding window. Login (`/auth/login`), emergency release (`/admin/emergency-clear`), password change (`/actors/{id}/password` with the current password) and the PWA's Claude-login relay keep their own limits, as today. Old rows are pruned.
- **Lockout response:** `429` with `Retry-After`; the audit log records a lockout (`auth.locked_out`: handle, IP, until), never the password.
- **Client IP:** `RemoteAddr`. `X-Forwarded-For` is **ignored** unless the request comes from a configured trusted proxy (`COLONY_TRUSTED_PROXIES`, CIDRs, empty by default). Test that a forged header does not change the IP.
- **Allowlist:** `COLONY_LOGIN_ALLOWLIST` (comma-separated IPs or CIDRs). A request from an allowlisted address is **never locked out** (the password is still checked, every failure is still audited). Empty by default in production.
  - **Dev profile only:** `deploy/compose.dev.yaml` and `deploy/.env.example` set it to Norbi's machine: localhost (`127.0.0.1/32`, `::1/128`), the address the dev container actually sees for his browser (find it out: in Docker it is usually the bridge gateway, not 127.0.0.1; write the finding in the report), and his PC's Tailscale address `100.122.147.16/32`. The production `deploy/compose.yaml` does not set it.
- **Status page (round 35):** a "Biztonság" card: lockouts in the last 24 hours, the allowlist (addresses only), whether trusted proxies are set.
- **Tests:** a restart keeps the lockout (memory store reloaded, Postgres in `test-pg`); per-IP and per-handle windows; the allowlist exempts; a forged `X-Forwarded-For` is ignored; `Retry-After` is set.

## 2. Dependency hygiene

- `make vuln`: `govulncheck ./...` and `npm audit --omit=dev` in `web/`; document it in `CLAUDE.md`'s command table. Pin the `govulncheck` version in the Makefile (a version that supports the current Go; v1.9.0 works with Go 1.27).
- `.github/dependabot.yml`: weekly updates for Go modules, npm (`web/`), and the Dockerfile base images, grouped so they come as few pull requests.
- The dev-only npm findings (`minimatch` through the ESLint packages): update if a fixed version exists without breaking `make test-web`; otherwise list them in the report as dev-only.
- A one-off secret scan of the whole git history (e.g. `gitleaks detect`, run locally, not installed globally): report what it finds; never print a secret in the report (type, file, commit only). If it finds a real secret, stop and tell Norbi first.

## 3. Container hardening (only after Norbi approves it in the chat)

What it means: limiting what an attacker could do **inside** the server container if they ever got code execution there. It has nothing to do with which ports are open. In `deploy/compose.yaml` (the server service only, not the shared services):

- `read_only: true` with `tmpfs: /tmp` (the files volume stays writable);
- `cap_drop: [ALL]` (the server needs no Linux capabilities to listen on 8080);
- `security_opt: [no-new-privileges:true]`;
- `mem_limit` / `pids_limit` sized from the live usage (Claude Code measured about 110 MB for the shared Postgres; measure the server on the dev stack under load first).

Before any of this, **ask Norbi in the chat**, with what each line prevents. If he says no, skip the part and say so in the report. Test on the dev stack that the server, the uploads, the migrations and the backups still work.

## 4. A full review against the standards

Review every module and the guard against **OWASP ASVS 4.0 level 2** (the sections that apply: V2 authentication, V3 sessions, V4 access control, V5 validation and encoding, V7 errors and logging, V8 data protection, V9 communication as far as it applies without HTTPS, V12 files, V13 API, V14 configuration) and the **OWASP Top 10 for LLM applications (2025)** (prompt injection, sensitive information disclosure, supply chain, excessive agency, improper output handling, system prompt leakage, vector and embedding weaknesses for round 36).

- Write `docs/14-biztonsagi-ellenorzes.md` (Hungarian): per requirement, **meets / partly / not applicable / does not meet**, with the file and line that shows it, and for every "does not meet" either the fix commit or why it stays (a decision for Norbi).
- **Fix** what you can within the decisions above, each with a test. Severity first. Typical places to look: IDOR (another workspace's or person's ID in every path parameter), mass assignment in PATCH bodies, error messages that leak internals, upload names and types, the guard's handling of agent output (paths, commands, file links), the MCP tools' argument validation, the WebSocket replay filter, the CSV and Markdown outputs, log lines with tokens or message bodies.
- **Fuzz tests** (Go native fuzzing, short default run in `make test`, long run documented) for the parsers that take outside input: message mention and chip parsing, the guard's stream-json readers (Claude Code, agy), the `/usage` parsers, file-link and path checks, the CSV export's formula guard, the token and handle validators.
- Anything that needs Norbi (an excluded item turns out worse than thought, a fix would change how agents work): stop and ask in the chat.

## Out of scope

HTTPS and HSTS; a non-root PC guard; removing `bypassPermissions` on the homelab; the homelab's LAN ports; a penetration test of the live server; ISO 27001 or SOC 2 (organisational standards, not applicable to this project today).

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (the limits, the allowlist, the headers, the deadlines), `docs/06-allapot-es-uzemeltetes.md` (what to do on a lockout, how to run `make vuln`), `docs/14-biztonsagi-ellenorzes.md`, `task.md`.

## Report

`docs/handoff/40-security-report.md` (Hungarian): the commits; the ASVS and LLM Top 10 result in one table (counts per status); every fixed finding with its severity and test; what stays open and why; the dev allowlist finding (which IP the container sees); the secret scan result; the deploy order; what the reviewer should look at first.
