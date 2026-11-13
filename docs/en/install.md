# Installing Colonyhall

The installer's design and its decisions are in `docs/15-telepites-terv.md` (Hungarian; English summary: [`docs/15-telepites-terv.en.md`](../15-telepites-terv.en.md)). This page is the user guide; the Hungarian original is [`docs/16-telepites.md`](../16-telepites.md).

## In short

```sh
curl -fsSLO https://github.com/badzso-boop/colonyhall/releases/latest/download/install.sh
less install.sh          # read it
sh install.sh            # downloads and verifies colony, then starts `colony setup`
```

`colony setup` asks what this machine should be: the **server**, a **guard**, or **both**. Before every step it shows what it would run; it uses `sudo` and installs software only after you confirm.

## The server

- **Folder:** `~/.local/share/colonyhall/server/` (`compose.yaml`, a `0600` `.env`; on macOS also `backup.sh`).
- **Database:** its own Postgres in the Compose stack by default, or an existing Postgres by connection URL (the question is hidden, because the URL contains a password). Inside the container `localhost` is the container itself: give the host's address or a Docker network name. The bundled Postgres is the `pgvector/pgvector:pg17` image (search uses the `vector` extension). **With an existing Postgres** the `vector` extension must be available, and if Colonyhall's database user is not a superuser, create it once: `CREATE EXTENSION vector;` in the database (otherwise the first start stops with this advice).
- **Address:** `127.0.0.1:8080` by default. You can choose the LAN or the Tailscale address. `0.0.0.0` only after an extra confirmation; unattended, only with `allow_all_interfaces: true`.
- **Docker on Linux runs with `sudo`:** membership of the `docker` group is equivalent to root, so the installer does not add you to it.
- **The first owner:** user name, display name and password (hidden, asked twice; leave it empty to generate one, printed once). The password reaches `colonyhall bootstrap` on stdin, never on a command line.
- **Backups (optional):** every night at 03:30 a `pg_dump` and an archive of the files volume, kept for 14 days.
  - **Linux:** a system-wide systemd timer running as root (because Docker runs with `sudo`), so everything it uses is owned by root: the script is `/usr/local/lib/colonyhall/backup.sh`, the backups go to `/var/backups/colonyhall/` (`0700`). Root never runs a script or writes a folder that your user can change, otherwise your user (or anything running as you, such as an agent) could become root. The script does not read your `compose.yaml` or `.env` either: it finds the database container by its Compose labels.
  - **macOS:** a launchd agent running as you; backups go to `~/colonyhall-backups/`.
  - **Restore:** `sudo docker compose … exec -T db pg_restore -U colony -d colonyhall --clean < colonyhall-YYYYMMDD-HHMM.dump`, and the files: `sudo docker run --rm -v colonyhall_files:/data -v /var/backups/colonyhall:/backup busybox tar xzf /backup/files-….tar.gz -C /data` (on macOS `~/colonyhall-backups` and no `sudo`).
- **Database role:** a **new** install with the bundled Postgres connects as a separate `colony_app` role (not a superuser, `NOBYPASSRLS`, so row-level security applies to it); its password is in `.env` (`COLONY_APP_DB_PASSWORD`), handed to `psql` on stdin. Installs made before this keep their superuser connection when upgraded; the server then warns in its log.
- **Upgrade:** `colony setup --server --upgrade` (the new image tag into `.env`, `pull`, `up -d`; migrations run on start). Upgrade the server first, then the guards.
- **Source link (AGPL section 13):** if you run a modified version, set `COLONY_SOURCE_URL=<where your modified source is>` in `.env`; the web app and `GET /api/v1/version` point there. Empty means the upstream repository.
- **Uninstall:** `colony uninstall --server` (the data stays); with `--purge` the data is deleted too.

### Login limits and the container

- After 5 failed passwords for one handle, or 20 from one IP address, within 15 minutes the server refuses further attempts for a while (`429` with `Retry-After`); the emergency release and the password change count separately. Lockouts are audited and shown on the System status page ("Security"). **Locked yourself out?** Wait for `Retry-After`, or add your address to `COLONY_LOGIN_ALLOWLIST` in `.env` (comma-separated IPs or CIDRs; failures are still audited) and run `docker compose up -d`.
- Behind a reverse proxy, list the proxy in `COLONY_TRUSTED_PROXIES`; otherwise `X-Forwarded-For` is ignored.
- The server container runs with a read-only root, no capabilities, `no-new-privileges`, and memory (1 GB) and process (256) limits: `COLONY_MEM_LIMIT`, `COLONY_GOMEMLIMIT`, `COLONY_PIDS_LIMIT`. `colony setup --server --upgrade` adds these lines to older installations.

### Several customers on one server (multi-tenant mode)

Off by default. When on, every workspace has its own subdomain (`<slug>.<base domain>`). Details: `docs/02-muszaki-specifikacio.md` and `docs/23-elkulonites.md` (Hungarian).

- `.env`: `COLONY_MULTITENANT=1`, `COLONY_BASE_DOMAIN=colonyhall.example.com`; the subdomains need wildcard DNS (`*.colonyhall.example.com`) and, with HTTPS, a wildcard certificate. The server's database role must not be a superuser and must not have `BYPASSRLS` (otherwise it refuses to start).
- A workspace's slug: `colonyhall bootstrap -slug acme …`; later `colonyhall set-slug -workspace <name> -slug acme`.
- Its own Gitea for one customer: `printf '%s\n%s\n' "$TOKEN" "$SECRET" | colonyhall gitea-set -workspace <name> -url https://git.acme.example` (a read-only token); that Gitea's webhook is then `https://acme.<base domain>/api/v1/hooks/gitea`. Switch it off with `colonyhall gitea-set -workspace <name> -off`.
- **Trying it locally without a domain:** browsers send `*.localhost` to your own machine. `COLONY_MULTITENANT=1 COLONY_BASE_DOMAIN=localhost`, two workspaces (slugs `acme` and `beta`), then `http://acme.localhost:8080/app/` and `http://beta.localhost:8080/app/`. From the command line: `curl -H 'Host: acme.localhost' http://127.0.0.1:8080/api/v1/me`.

### HTTPS (a recipe; not built in)

The installer does not set up TLS. If the server must be reachable from outside your home network or Tailscale, put it behind a reverse proxy, for example Caddy (with automatic certificates):

```
colony.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Then set `COLONY_SECURE_COOKIES=1` in `.env` and run `colony setup --server` (or `sudo docker compose … up -d`). Keep the server on `127.0.0.1`; only the proxy faces the outside.

## The guard

- **Pairing the machine:** in the web app, **Machines → New machine** gives a pairing code and the install command; in "both" mode the installer asks for a code itself, as the owner it just created.
- **`~/.config/colonyhall/guard.yaml`,** with comments: the root folders (default `~/projects`; **on WSL it does not offer the `/mnt/*` Windows drives**), the sandbox (bubblewrap on Linux), `allow_unsandboxed`, `max_concurrent_runs: 2`, `preview_ports`, `claude_token_file`.
- **The service:**
  - Linux and WSL: a systemd user unit (`~/.config/systemd/user/colony-guard.service`) and `loginctl enable-linger` (after you confirm, with `sudo`), so it runs while you are logged out.
  - WSL without systemd: after you confirm, `[boot] systemd=true` goes into `/etc/wsl.conf`; then `wsl --shutdown` in Windows, reopen the terminal and run `colony setup` again to continue.
  - WSL keepalive (optional): a Windows task at logon (`conhost.exe --headless wsl.exe -d <distribution> -- sleep infinity`). Without it the guard only runs while a WSL terminal is open.
  - macOS: a LaunchAgent (`~/Library/LaunchAgents/hu.colonyhall.guard.plist`), logs in `~/Library/Logs/colonyhall/`. macOS has no sandbox yet: only full-access agents run there, and the server refuses sandboxed ones.
- **AI clients:** the installer downloads the chosen client's official installer, prints its address, size and SHA-256, shows it on request, and runs it only after you confirm. **Logging in is your job** (`claude`, `agy`), or give the web app your Claude token (Machines → Claude login). Installing Antigravity is not in the installer yet.
- **A colleague on the same machine:** a separate Linux user, and `colony setup --guard` as that user (their own guard, their own subscription).
- **Upgrade:** `colony upgrade` (download, verify, swap, the previous binary stays as `colony.prev`, restart, doctor). **Uninstall:** `colony uninstall` (the config stays), or `--purge`.

## Unattended install

```sh
colony setup --guard --yes --server-url http://100.64.0.10:8080 --pair-code K7Q2-MX4P --name laptop --roots ~/projects
colony setup --answers setup.yaml --yes
```

An answers file (`chmod 600` if it holds a secret; better use the `COLONY_ADMIN_PASSWORD` and `COLONY_DATABASE_URL` environment variables):

```yaml
mode: both
server:
  bind: 127.0.0.1
  port: 8080
  admin_handle: anna
  admin_name: Anna
  backups: true
guard:
  name: laptop
  roots: [/home/anna/projects]
  allow_unsandboxed: true
  max_concurrent_runs: 2
  linger: true
  clients: [claude]
```

`--dry-run` prints the plan (commands and files, no secrets) and changes nothing. Running it again skips the finished steps; after a failure it continues where it stopped. Log: `~/.local/state/colonyhall/setup.log` (secrets masked).

## Troubleshooting

- `colony doctor` (or `--json`): the server, the machine token, `guard.yaml`, the clients and their login, the sandbox, tmux, git, the service, free disk space, each with its fix. The Machines page shows the same per machine.
- Guard log: `journalctl --user -u colony-guard -f` (Linux, WSL), `~/Library/Logs/colonyhall/hu.colonyhall.guard.log` (macOS).
- Server log: `sudo docker compose --project-directory ~/.local/share/colonyhall/server logs -f server`.
