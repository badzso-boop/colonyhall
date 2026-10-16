# Handoff prompt 13: previews — run an agent's app and share it by link

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/13-preview-prompt.md)"
```

---

Branch: `feature/preview`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 7, and the section "Előnézet (preview)". This task runs containers on the user's machine through the guard: be strict, test every refusal. In tests use a fake docker runner; **do not start real containers** except, at most, one manual smoke test that you clean up and describe in the report.

## Server (new module `preview`)

- Table `preview` (`id`, `workspace_id`, `agent_id`, `host_id`, `channel_id`, `thread_root_id`, `name`, `compose_path` (relative to the agent's root/worktree), `state` (`requested` | `running` | `stopped` | `failed`), `url`, `error`, `expires_at`, timestamps; state changes by conditional updates, events for audit).
- MCP for agents: `preview_start` (name, compose file relative path, ttl_minutes ≤ 24 h, default 4 h) and `preview_stop` (name). Requires a per-agent assignment flag `allow_preview` (default false; contract-first, migration, dev page).
- The host config (`GET /api/v1/host/config`) carries the agent's requested/running previews; the guard reconciles them (start requested, stop expired/stopped). `POST /api/v1/host/previews/{id}` lets the guard report `running` + URL or `failed` + reason.
- REST for humans: list previews, stop one. Emergency stop and pausing the agent stop its previews.

## Guard

- Validates the compose file **before** running anything: parse YAML; refuse `privileged`, `cap_add`, `devices`, `network_mode: host`, `pid: host`, `ipc: host`, `userns_mode`, `security_opt`, any volume or bind mount outside the agent's root/worktree (resolve symlinks), any mount of the Docker socket, `build` contexts outside the root; require `deploy.resources.limits` (cpus, memory) or apply defaults via an override file.
- Ports: the agent's compose may declare container ports; the guard publishes them itself from a per-agent range (config `preview_ports: 41000-41999`, 10 per agent) on the machine's Tailscale or configured address (`preview_host` in `guard.yaml`), through a generated override file — the agent's own `ports:` mappings are replaced, never trusted.
- Runs `docker compose -p colony-<handle>-<name> -f <compose> -f <override> up -d` (argument slice, timeout), outside the sandbox; stops with `down` on stop/expiry/halt. Reports the URL (`http://<preview_host>:<port>`), the server posts it into the thread.
- Tests with a fake runner: every refusal rule, port allocation and reuse, override content, stop on expiry/halt/emergency, reconcile after a guard restart.

## Dev page

"Előnézetek" list (in the "Gépek" panel or its own): agent, name, link, state, expires, stop button.

## Docs

`docs/02-muszaki-specifikacio.md` (Előnézet paragraph), plan section "Előnézet" marked, `CLAUDE.md` schedule. Hungarian.
