# Installing Colonyhall (design summary)

Status: proposal, 2026-11-05. The full design (Hungarian) is `15-telepites-terv.md`. This summary is the base of the public README's install section.

## What you install

- **The server**: one Docker Compose stack (the Colonyhall server image and, by default, its own Postgres). Runs on one machine.
- **The guard** (`colony`): one per machine that runs AI agents (Claude Code, Antigravity). It wakes the agents, runs them in a sandbox where available, and reports to the server.

Linux (Debian/Ubuntu, Fedora; others best effort), WSL2 and macOS (Intel and Apple silicon). No Go toolchain needed: prebuilt binaries from GitHub Releases, verified against `SHA256SUMS` and build provenance attestations.

## Install

```sh
curl -fsSLO https://github.com/<owner>/colonyhall/releases/latest/download/install.sh
less install.sh        # read it first
sh install.sh          # downloads colony, verifies it, then runs `colony setup`
```

`colony setup` asks whether this machine is the **server**, a **guard**, or **both**, and then:

1. lists missing dependencies (Docker for the server; git, tmux, bubblewrap on Linux for the guard) and installs them with your package manager **after you confirm**;
2. for the server: writes `compose.yaml` and an `.env` with generated secrets (`0600`), binds to `127.0.0.1` unless you choose a LAN or Tailscale address, starts it, and creates the first owner account;
3. for the guard: pairs the machine with a code from the Machines page, writes a commented `guard.yaml`, and installs a systemd user service (Linux, WSL) or a launchd agent (macOS);
4. checks whether your AI clients are logged in. **Logging in is always your job**: run `claude` (or set your own `claude setup-token` token in the web app) and `agy` as the same user; the installer never asks for an AI account password.

Options: `--yes` and flags or `--answers file.yaml` for unattended installs, `--dry-run` to see the plan without changing anything. Running it again repairs or does nothing. English by default, Hungarian when your locale is Hungarian.

## Afterwards

- `colony doctor`: checks the server connection, the token, the clients and their login, the sandbox, tmux, the service and disk space, and tells you how to fix what fails. The Machines page shows the same.
- `colony upgrade`: downloads and verifies the new version, swaps it, restarts the guard. Server: `colony setup server --upgrade` (migrations run on start).
- `colony uninstall` (`--server` for the server): removes the service and binaries, keeps your data and config unless you pass `--purge`.

## Platform notes

- **macOS:** no sandbox yet; only full-access agents run there, and the server refuses sandboxed agents on such a machine.
- **WSL:** the guard needs systemd (the installer can enable it; then `wsl --shutdown` once). Optionally the installer adds a Windows logon task that keeps the distro running; without it, the guard runs only while a WSL terminal is open.
- **TLS:** not built in. Keep the server on localhost, a LAN or Tailscale, or put it behind a reverse proxy with HTTPS.
