# Handoff prompt 38: a colleague with their own agents and their own Claude subscription

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/colleague ../new-chat-worktrees/colleague main
cd ../new-chat-worktrees/colleague
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/38-colleague-prompt.md)"
```

---

Branch: `feature/colleague`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/colleague`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. Next free migration: check `migrations/` (00046 at the time of writing).

After each part: `gofmt`, `make test`, `make lint`; `make test-web` for PWA parts; `make test-pg` when a store changed. At the end `go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it.

**Do not touch the homelab** (no user creation, no guard install, no config change there). Part 5 is a runbook Norbi executes; you write and test it locally.

## Why (Norbi, 2026-11-04)

Norbi takes on one colleague (code and documentation). The colleague:

- uses **their own Claude Code subscription**, never Norbi's (Norbi does not share his; a Team plan may come later, API keys are too expensive);
- gets agents that **Norbi creates and assigns** as admin;
- the agents **run on the homelab** (its capacity is small, enough for testing; it grows later);
- sees channels exactly like an agent does: only those Norbi adds them to, with **read-only or read-write** membership per channel;
- **does not see Norbi's agents** (their overview, runs, steps, costs).

Two agents per colleague, both on the homelab:

1. **A full-access "planner" agent:** reads the colleague's project folders, **may only write `.md` files**, cannot run commands, and delegates coding to the sandboxed agent. The restriction is **enforced by Claude Code's permission settings**, not only by the prompt.
2. **A sandboxed coding agent:** the existing bwrap sandbox with the colleague's granted folders.

## Norbi's decisions (binding)

1. **Separate Linux user** on the homelab for the colleague (e.g. `colleague`): no `sudo`, not in the `docker` group, only their own folders. **A second guard** runs as that user and appears in Colonyhall as its own machine (e.g. `homelab-colleague`). Reason: Norbi's homelab user has passwordless `sudo` and Docker access, so an agent running as him could reach every project on the server.
2. **Login with `claude setup-token`:** the colleague runs it on their own computer and gets a long-lived token for **their** subscription. They enter it **themselves, in the PWA**; Norbi never sees it. It reaches only the colleague's guard.
3. **Planner agent permissions:** enforced by Claude Code permission rules (no Bash; writes only `*.md`), plus the prompt that says to delegate code.
4. **The Runs page filters by person.**

## 1. Agent owner and visibility

- **Server** (contract first; migration): an agent gets an optional **owner** (a human actor), set at creation (`POST /agents`, admins only) and changeable by the owner of the workspace only (`PATCH`, audited, event). Machines (hosts) get an optional owner too (the colleague's guard belongs to the colleague).
- **Visibility for a member:**
  - Agents page, agent overview, runs, steps, errors (`/errors/{id}`), usage, terminals, file browser: **only their own agents** (owner = them). Everything else 404 (never reveal that another agent exists beyond what channel membership already shows: an agent's name and avatar in a shared channel stay visible).
  - Admins and the owner see everything, as today.
- **Giving tasks:** a member may give tasks (mention, DM) only to their own agents and to sandboxed agents in channels they can write to. A member can never wake a full-access agent that is not theirs (today's rule "only a human gives a full-access agent a task" becomes "only its owner or an admin").
- **Runs page:** a "Person" filter (owner of the agent) next to agent/machine/channel; `GET /runs` and `GET /usage` get `owner_id` (additive). For a member, the server always narrows to their own agents, whatever the filter says.
- **Tests:** a member gets 404 for another person's agent overview, runs, error and terminal; sees their own; cannot DM-wake Norbi's full-access agent; the Runs filter by person.

## 2. Channel read and write permissions

- Membership already has `can_write` (`policy.CanWriteChannel`, the PWA's read-only composer). **Missing:** changing it for an existing member.
- **Server:** `PATCH /channels/{channelId}/members/{actorId}` with `can_write` (admins; audited; event `channel.member_updated`; silent note "@x mostantól csak olvashatja / írhatja a csatornát").
- **Audit every write path** against `can_write` and add tests where one is missing: message, thread reply, reaction, upload, retract, decision and wiki writes bound to the channel, mentions that wake agents, file fetch requests, terminal starts, schedules posting into the channel.
- **PWA:** in the channel settings Members tab (round 34), a "Csak olvashat" / "Írhat" switch per member, and the same choice when adding members.
- **Tests:** a read-only member gets 403 on each write path; switching to write works without re-adding.

## 3. Docs-only planner agent (enforced)

- **Agent config** (immutable after creation, rule 8): a new mode for full-access Claude Code agents, e.g. `write_scope: "docs"` (default `"all"`). Only for `claude-code` with full access; the server refuses other combinations.
- **Guard:** for a docs-only agent, start Claude Code with permission rules that:
  - deny `Bash` (and any other command or notebook tool);
  - allow `Write` and `Edit` only for `**/*.md`;
  - keep reads inside its root folders.
  Use `--settings` (session-only) or the permission flags; find out with the real CLI which rule order Claude Code applies (deny vs allow precedence) and **prove it**: a test against the fake runner for the arguments, plus a recorded manual check in the report that a non-`.md` write and a `bash` call are refused and a `.md` write works (Claude Code 2.1.x on the PC, in a scratch folder, nothing else touched).
- **Refused actions are refusals, not approval questions:** they must not turn into "approve in chat" prompts (`--permission-prompt-tool`), or the restriction is one click away.
- **Prompt:** the agent's own instructions get a fixed block (in English, inside the instructions, not editable away by the agent): it plans and documents, writes only Markdown, and hands every code change to its sandboxed partner (named at creation) with a clear task; it never asks a human to run code for it.
- **PWA:** in the create-agent wizard (round 33), "Csak dokumentációt írhat (kódot a sandboxos párja ír)" with the partner picker; the agent's profile shows the mode.
- **Tests:** the guard arguments for a docs-only agent; the server refuses the mode for a sandboxed or Antigravity agent; the prompt block is present.

## 4. The colleague's Claude login from the PWA

The guard already reads one token file (`claude_token_file`, used only for sandboxed runs). Extend that:

- **Guard:** the token is used for **all** Claude Code runs of that guard (direct, work, decide, the usage check), as `CLAUDE_CODE_OAUTH_TOKEN` in the child environment only. The guard's own login file is never copied into the sandbox when a token is set. The file is `0600`, in the guard's config folder, and stays in the deny list (agents never read it).
- **Setting it from the PWA:**
  - The Machines page shows, for a machine **owned by the viewer** (or for admins): "Claude-belépés: beállítva (2026-11-04) / nincs". Owner of the machine = the colleague (part 1).
  - "Belépés beállítása": a Dialog that explains `claude setup-token` (run it on your own computer, copy the token), with a password field.
  - The token goes `PWA → server → that machine's guard` over the existing host WebSocket relay (the pattern of the file-system requests, round 20) and is **never stored on the server**, never logged, never in an event, an audit payload or an error detail. The server records only `host.claude_login_set` (who, when, which machine).
  - The guard checks it before saving: one `claude -p /usage --output-format json --no-session-persistence --strict-mcp-config` with the new token (no model call) must succeed; otherwise it answers with an error and keeps the old one. The Dialog shows the result.
  - If the guard is offline: 409, "a gép nem elérhető", nothing stored anywhere.
  - "Belépés törlése" removes the file (owner or admin).
- **Who may set it:** the machine's owner and admins. Never an agent. Rate limit the endpoint.
- **Tests:** the relay round trip; the token absent from server logs, events, audit and DB (search them in the test); refused for another person's machine and for agents; a failing check keeps the old token; offline 409.

## 5. A second guard under its own Linux user (runbook + support)

- **Runbook** (`docs/10-kollega-beallitasa.md`, Hungarian), for Norbi to run on the homelab, step by step:
  - create the user without `sudo` or `docker` group, with its own home and project folder;
  - install the `colony` binary for that user, `guard.yaml` (roots in the user's home only, `claude_token_file` path, its own `sandbox_dir`, state folder, preview ports that do not clash with Norbi's guard), `loginctl enable-linger`, systemd user service;
  - enroll as machine `homelab-colleague`, set the machine owner;
  - create the two agents and the channel memberships;
  - the colleague enters the token (part 4);
  - how to remove it all.
  Read `~/homelab/CLAUDE.md` and `README.md` rules from the repo docs you have (`docs/05-homelab-telepites.md`); do not log in to the homelab.
- **Guard support:** make sure two guards on one machine under different users do not clash: tmux socket names, the sandbox proxy and web-read sockets, preview ports, state and sandbox folders, the usage-check folder. Test with two supervisors in one test process (different temp homes).
- **Concurrency on a small machine:** `max_concurrent_runs` in `guard.yaml` (default unlimited): runs beyond it wait (items stay pending, no attempt counted), with a silent note "⏳ sorban állok, a gép most foglalt" once per thread.

## 6. PWA

- Admin: Users page: the owner column of agents (count); agent creation: owner picker (humans), docs-only option (part 3); channel members: read/write (part 2); Runs: person filter (part 1); Machines: owner and the Claude login state (part 4).
- Member: the Agents page and the sidebar list only their own agents; the Machines page only their own machine, with the login Dialog.

## Out of scope (say so in the report)

Creating Linux users or installing anything on the homelab (Norbi runs the runbook); Antigravity logins per user; the Team plan; resource quotas per person beyond `max_concurrent_runs` and the existing per-agent daily token and session limits.

## Docs

Hungarian:

- `docs/01-terv.md`: the colleague model (owner, visibility, own subscription, docs-only planner + sandboxed coder, separate Linux user);
- `docs/02-muszaki-specifikacio.md`: endpoints, fields, events, the token relay, the permission rules;
- `docs/06-allapot-es-uzemeltetes.md`: where to look when the colleague's agent does not run;
- `docs/10-kollega-beallitasa.md`: the runbook;
- `task.md`.

## Report

`docs/handoff/38-colleague-report.md` (Hungarian), with:

- the commits;
- the manual permission check (part 3) with its output;
- the token's path through the system, and the evidence that it is stored nowhere on the server;
- the runbook's dry run (what you could test locally);
- the deploy order (server, then all guards; the new guard only after the runbook);
- what the reviewer should look at first: the visibility filters (part 1), the token relay (part 4), the docs-only permission rules (part 3).
