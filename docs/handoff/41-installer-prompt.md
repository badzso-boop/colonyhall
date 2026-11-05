# Handoff prompt 41: an installer for the whole project (server and guards)

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/installer ../new-chat-worktrees/installer main
cd ../new-chat-worktrees/installer
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/41-installer-prompt.md)"
```

---

Branch: `feature/installer`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/installer`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push, **never create a GitHub release or tag** (you prepare the workflow; Norbi runs the first release).

After each part: `gofmt`, `make test`, `make lint`, and `shellcheck` on every shell script. **Commit only when they pass.** Every fix needs a test that fails without it.

**This round has a design gate.** Part 0 ends with a design document, and **you stop there** until Norbi approves it in the chat. Parts 1–7 start only after that.

**Never touch Norbi's machines' real setup** (his PC's guard, the homelab, nato, their `guard.yaml`, services or packages). Test in throwaway containers and VMs, or in a scratch `HOME`.

## Why (Norbi, 2026-11-05)

Today Colonyhall is installed by hand, from notes spread over `CLAUDE.md`, `docs/05`, `docs/10` and the handoffs. Norbi wants to install it on new machines and for colleagues, and above all **later for other people**: he may open-source it or sell it as a subscription. So the installer must be **polished and general**: a person who has never seen the project installs the whole thing on their own machine and gets a working system, with nothing specific to Norbi's homelab.

## Decisions (binding)

- **Platforms, first round:** Linux (Debian/Ubuntu, Fedora; others best effort), **WSL2**, **macOS** (Intel and Apple silicon).
- **Scope: everything.** The server (as if a new person installs the whole project) **and** the guard on each machine.
- **Distribution: GitHub Releases, with a token** while the repository is private (it may become public later: the installer must work without a token then). Prebuilt binaries, no Go toolchain needed on the user's machine.
- **Dependencies:** the installer lists what is missing and **installs it automatically after the user confirms** (with the system package manager, `sudo` when needed). Logging in to the AI clients is **always the user's job**: both the installer and the guard's own messages say so clearly, and say how.
- **WSL keepalive:** offered as an option. If the user wants it, the installer sets it up (a Windows startup entry that keeps the distro running, as Norbi has on `nato`). If not, it explains that the guard runs only while a WSL terminal is open.
- **macOS:** no sandbox in the first round. Only full-access agents run there; the server knows it (the host report already says whether a sandbox is available) and refuses sandboxed agents on such a machine with a clear message. A macOS sandbox comes later.

## 0. Design gate

Write `docs/15-telepites-terv.md` (Hungarian), and a short English summary for the future public README. Answer:

1. **The pieces and the order** for a new user: download, verify, server, first admin, open the PWA, add a machine, guard install, AI client login, first agent. What can be skipped (a guard-only install against an existing server; a server-only install).
2. **Releases:**
   - a GitHub Actions workflow that builds `colony` and the server for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`;
   - the server image for GHCR;
   - `SHA256SUMS` and build provenance attestations;
   - version numbers (SemVer, `colony --version` and the server's `Version` set by `-ldflags`).
   - How a private repo's assets are downloaded with a token (a fine-grained, read-only token; from `GITHUB_TOKEN`, `gh auth token`, or asked with hidden input; **never stored, never logged, never on a command line that ends up in shell history**), and that the same script works without a token for a public repo.
3. **The installer's shape:**
   - a small POSIX `install.sh` that only detects the platform, downloads the right `colony` binary and verifies its checksum;
   - then **`colony setup`** (Go, testable, translatable) does the real work. Say why this split.
   - Interactive by default; `--yes` and flags or an answers file for unattended installs; `--dry-run` prints the plan and changes nothing; idempotent (running it again repairs or does nothing); a log file; resumable after a failure.
   - **Languages:** English by default, Hungarian when the locale says so.
4. **The server install:**
   - Docker and Compose (install Docker after confirmation on Linux; on macOS point to Docker Desktop or OrbStack, which the user installs);
   - its own Postgres by default, or an existing one (a connection URL, like Norbi's shared homelab Postgres);
   - generated secrets in an `.env` (`0600`);
   - the listen address (localhost only by default; LAN or a Tailscale address when the user chooses; never `0.0.0.0` without saying what it means);
   - the first admin (the owner) and their password (asked hidden, or generated and shown once);
   - optional Gitea; backups (what to back up, a simple scheduled `pg_dump` option);
   - upgrades (pull the new image, migrations run on start), uninstall (keeps the data unless asked).
5. **The guard install:**
   - enroll with a pairing code from the Machines page;
   - writing `guard.yaml` with commented defaults from the user's answers (roots, exchange folder, sandbox, preview ports, `max_concurrent_runs`). On WSL, `/mnt/*` Windows drives are not offered as roots by default; say what each choice means.
   - **the service:** systemd user unit plus `loginctl enable-linger` on Linux and WSL (enable systemd in `/etc/wsl.conf` after confirmation, with the needed `wsl --shutdown` explained); a `launchd` LaunchAgent on macOS;
   - the WSL keepalive option (above);
   - the colleague-style separate Linux user (round 38) as an advanced option, or a pointer to `docs/10-kollega-beallitasa.md`.
6. **Dependencies per platform** (check, list, install after confirmation): `git`, `tmux`, `bwrap` (Linux), `curl`, CodeGraph (round 39: its reviewed installer, telemetry off), the AI clients' own installers (Claude Code, Antigravity) when the user picks them. **Show the user what will run before it runs** (no blind `curl | sh` of third-party scripts: download, show the source and checksum, then run). Package managers: `apt`, `dnf`, `brew` (others: print the commands).
7. **AI client login:** what the installer checks (is `claude` logged in, is a `claude setup-token` token configured, is `agy` logged in) and the exact message it prints. It never logs in for the user, never asks for an AI account password.
8. **`colony doctor`:** checks the server connection, the token, the clients and their login, the sandbox, tmux, CodeGraph, the service state, disk space; prints fixes; the same results go into the host report for the Machines page.
9. **`colony upgrade` and `colony uninstall`** for the guard (download, verify, swap, restart the service; uninstall removes the service and the binary and keeps the config unless asked).
10. **Testing:** containers for the Linux distributions (Docker in Docker for the server part where possible), a WSL test plan (Norbi can run it on a spare distro), macOS (a GitHub Actions macOS runner, or a manual test list if runner minutes are a concern: say which and what it costs).

**Then stop** and tell Norbi in the chat what you propose and what he has to decide.

## 1. Releases

The workflow, version stamping, checksums and attestations, as approved. A `make release-local` that builds the same artifacts locally for testing.

## 2. `install.sh`

POSIX `sh`, `shellcheck` clean. Detect the OS and architecture, find the latest (or a pinned) release, download with or without a token, verify `SHA256SUMS`, put `colony` into `~/.local/bin` (or a chosen folder), then run `colony setup`. Tests: run it in containers against a local fake release server (no real GitHub calls in tests).

## 3. `colony setup`: the server

As designed. Tests with a fake Docker runner for the command sequence, and one real run in a container where Docker is available.

## 4. `colony setup`: the guard

As designed, including `guard.yaml` generation (golden-file tests per platform and answer set), the service on Linux, WSL and macOS (unit and plist content tests; `systemctl` and `launchctl` behind a runner interface), the WSL systemd and keepalive steps, the dependency installs behind a package-manager interface with fakes in tests.

## 5. `colony doctor`, `upgrade`, `uninstall`

As designed, with tests. The host report gets the doctor's result (contract first); the Machines page shows it.

## 6. The PWA

The Machines page's "Új gép" wizard (round 33) shows the right one-line install for the chosen platform with the pairing code, and the doctor's result per machine.

## 7. Documentation

- A new `README.md` for people who have never seen the project, in English: what it is, requirements, install in a few commands, first steps, upgrade, uninstall, troubleshooting. Norbi's homelab specifics stay in `docs/05` and `CLAUDE.md`.
- `docs/15-telepites-terv.md` (the approved design, Hungarian) and an install guide `docs/16-telepites.md` (Hungarian).
- Update `CLAUDE.md`'s commands and `docs/06-allapot-es-uzemeltetes.md`.

## Out of scope

A macOS sandbox (later); Windows without WSL; package repositories (apt, Homebrew tap) and signed macOS packages (`.pkg`, notarization): later, if it is published; a hosted, multi-tenant version.

## Report

`docs/handoff/41-installer-report.md` (Hungarian): the commits; what was tested on which platform and what only on paper; the first release checklist for Norbi (token, tag, what to check); what the reviewer should look at first (secret handling in the scripts, what runs with `sudo`, the downloaded third-party installers).
