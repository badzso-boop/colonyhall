# Handoff prompt 07: everyday use — guard service, "new agent" form, secrets exchange

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/07-everyday-prompt.md)"
```

---

Branch: `feature/everyday`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 1.

## 1. `colony guard install`

- New subcommand in `cmd/colony`: `colony guard install -config <path to guard.yaml>`. It validates the config (`guard.LoadConfig`), resolves the absolute paths of the `colony` binary and the config, and writes a systemd **user** unit to `~/.config/systemd/user/colony-guard.service` (`Restart=on-failure`, `RestartSec=10`, `WorkingDirectory` = the config's folder, no secrets in the unit). It prints the commands the user runs to enable it (`systemctl --user daemon-reload && systemctl --user enable --now colony-guard`, and `loginctl enable-linger $USER` for running without a login) — it does **not** run them. `-print` writes the unit to stdout instead of the file. Refuses to overwrite an existing unit unless `-force`.
- Unit tests for the generated unit text (paths quoted correctly, spaces in paths).
- Update `guard.example.yaml`: a commented host-mode example with real-looking roots (`/mnt/g/Projects`), `allow_unsandboxed: true` explained, and the new `exchange_dir`.

## 2. "Új agent" form

- On the "Gépek" panel (admins): a form "Új agent": handle, display name, client (`claude-code`/`antigravity`), the channels to join (checkboxes from the channel list), and optionally the host + assignment fields (mode, root dir, model, permission mode, approvals) to assign it right away. It calls the existing endpoints (`POST /api/v1/actors`, `POST /api/v1/channels/{id}/members`, `PUT /api/v1/agents/{id}/host`) in order and shows errors per step. The token the actor creation returns is not needed in host mode — do not display it (the guard requests its own tokens); say so in a short note.
- If the existing endpoints miss something the form needs, extend them contract-first, with e2e tests.

## 3. Secrets exchange folder

- `guard.yaml` (host mode): `exchange_dir` (optional; `~` expanded). On start the guard checks it exists and is a directory it can write (create a temp file and remove it); it reports `exchange_dir` and `exchange_ok` in the host report (OpenAPI `HostReport`, `hosts.Report`, shown on the "Gépek" panel). If not ok, log a warning (e.g. the Z: drive is not mounted: `mount -t drvfs Z: /mnt/z`).
- Direct-mode prompt (`DirectPrompt` or where the guard builds the run prompt): when `exchange_dir` is ok, add one short paragraph: secrets go through `<exchange_dir>/<project>/` with mode 600, the chat gets only the path, the receiver copies and deletes it, never commit the exchange folder.
- Tests: report fields; prompt contains the paragraph only when the folder is ok; unwritable folder → `exchange_ok=false`.

## Docs

`docs/02-muszaki-specifikacio.md` (Őr section: install command, exchange_dir), `CLAUDE.md` (Parancsok table: `colony guard install`; schedule). Hungarian.
