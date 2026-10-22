# Handoff prompt 18: sandboxed agents get their own git worktree, and their network goes through an allowlist

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/sandbox-worktree-net ../new-chat-worktrees/sandbox-worktree-net main
cd ../new-chat-worktrees/sandbox-worktree-net
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/18-sandbox-worktree-network-prompt.md)"
```

---

Branch: `feature/sandbox-worktree-net`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/sandbox-worktree-net` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on `feature/sandbox-worktree-net`; never merge, never push.

## Why

Two items from the plan ("Később" in `CLAUDE.md`):

1. **Sandboxed agents have no git worktree.** Full-access agents get `<repo>-worktrees/<handle>` on branch `colony/<handle>/work` (round 12, `internal/guard/worktree.go`); the server refuses a `worktree_repo` for sandboxed agents (`ErrWorktreeDirectOnly`). A sandboxed agent that edits a repo writes straight into a granted folder, with no branch and no history.
2. **The sandbox shares the machine's network.** `bwrap` runs without `--unshare-net` (`internal/guard/sandbox.go`), so a sandboxed agent can reach anything the machine can: the internet, the homelab's other services (Postgres, Gitea, Home Assistant…), the LAN. It only needs its AI provider and, if configured, the Colonyhall server.

Read first: `CLAUDE.md`, `docs/01-terv.md` ("Gépek, mappahozzáférés és sandbox", "Munkaterület: git worktree agentenként"), `internal/guard/sandbox.go`, `internal/guard/worktree.go` (`gitExec`: hooks and fsmonitor off), `internal/guard/host.go` (`reconcile`/`policy`/`startAgent`), `internal/guard/guard.go` (`work`), `internal/guard/paths.go` (`CheckGrant`, `deniedHome`), `internal/guard/filefetch.go` (`isSecretName`), `internal/guard/config.go`, `internal/modules/hosts/service.go` (`ErrWorktreeDirectOnly`, assignment validation), `guard.example.yaml`.

Both parts are guard-heavy and security-critical; test them with the fake adapter and real `bwrap` where the test machine has it (skip cleanly where it does not). The homelab and the PC both have `bwrap`.

Do the parts in this order; one commit (or a few) per part; after each part `gofmt`, `make test`, `make lint`. Run `go test -race ./internal/guard/ ./internal/e2e/`. No database change is expected; if you need one, it is the next migration after `00025`.

## Decisions (Norbi, 2026-10-22; do not change them)

- **A sandboxed agent still runs no commands**, git included. **The guard does the git work for it**, outside the sandbox, with the hardened `gitExec`.
- **Commits stay local** on `colony/<handle>/work`. The guard does not push and sandboxed agents get no git credentials; a human (or a full-access agent asked by a human) reviews and merges or opens the PR.
- **Network: deny by default, allow by host name.** The sandbox gets its own network namespace with no route out; the only way out is an HTTP proxy run by the guard that allows a short list of `host:port`. It never allows the machine's private services.
- **Rollback switch:** `sandbox_network: open` in `guard.yaml` restores today's behaviour on that machine, without a new build.

## 1. Worktree for sandboxed agents

Server:

- Allow `worktree_repo` for sandboxed agents in `compact` mode (keep it refused where it makes no sense, e.g. `direct` mode with `sandbox: true` if that combination is invalid today — check the current validation and say what you kept). The same path rules as for full-access agents (`ValidatePath`, inside the machine's roots on the guard side).
- OpenAPI/UI: the assignment form shows the worktree field for sandboxed agents too, with a one-line hint ("a guard commits for the agent; nothing is pushed").

Guard:

- `reconcile`: for a sandboxed agent with `WorktreeRepo`, `EnsureWorktree` as for full-access agents (same `CheckGrant` on the repo, same refusal reporting). The work run's directory is the worktree; it is mounted **read-write** in the sandbox like a write grant, and the agent's other grants stay as they are.
- **Inside the sandbox the worktree's `.git` file is read-only** (`--ro-bind` it over itself): a worktree's `.git` is a pointer (`gitdir: …`); if a run could rewrite it, the guard's next git command outside the sandbox would follow it into a folder the agent controls. In addition, **every git command the guard runs for a sandboxed worktree passes `--git-dir=<repo>/.git/worktrees/<name>` and `--work-tree=<worktree>` explicitly** (computed by the guard from the repo, never read from the `.git` file), with the existing `-c core.hooksPath=/dev/null -c core.fsmonitor=false`, `GIT_CONFIG_NOSYSTEM=1`, and also `GIT_CONFIG_GLOBAL=/dev/null` so the user's global config (aliases, credential helpers, `includeIf`) does not apply.
- **After each work run** that changed files (`git status --porcelain`):
  - refuse to commit, and say so in the chat reply (one short line, details in the guard log), if a changed or new file matches the secret names (`isSecretName`), is larger than 10 MB, or is a symlink pointing outside the worktree;
  - otherwise `git add -A` and `git commit` with author `"<handle> (Colonyhall sandbox) <handle@colonyhall.local>"` and a message built by the guard: first line `colony(<handle>): <first 60 characters of the task>`, then the message link (`Requested in #channel by @who`), never the agent's free text as the subject (it is data, and could be crafted);
  - append to the chat reply one line: `🌿 commit <short sha> on colony/<handle>/work (<n> files)`.
- A run that changed nothing makes no commit. A failed commit never fails the run: the reply says the changes are left uncommitted in the worktree.
- The work prompt names the branch: "You work in a git worktree on branch colony/<handle>/work; the guard commits your changes after the run; you cannot run git or any command."
- The host report already carries worktrees; the agent panel (round 15) shows the branch for sandboxed agents too.
- Tests: worktree created for a sandboxed agent; the `.git` file is read-only inside a real `bwrap` run (skip without `bwrap`); a run that rewrote `.git` (simulate in the test by editing it from outside) does not redirect the guard's commit (explicit `--git-dir`); secret file → no commit + reply line; big file → no commit; symlink out → no commit; normal change → one commit with the guard's author and subject; no change → no commit; the global git config is ignored (set an alias/`core.hooksPath` in a temp `HOME` and show it has no effect).

## 2. Network allowlist for the sandbox

Design (no new external dependency; do not require `socat`, `slirp4netns` or root):

- `bwrap --unshare-net`: the sandbox gets its own network namespace with only `lo` (bwrap brings it up).
- The guard runs, per sandboxed run, an **HTTP proxy on a Unix socket** inside the agent's sandbox folder (outside the agent's writable dir, so a run cannot replace it), bound into the sandbox.
- Inside the sandbox the guard's **own binary** is the entry point: a hidden subcommand (e.g. `colony sandbox-bridge -sock <path> -port 3128 -- <client> <args…>`) listens on `127.0.0.1:3128`, forwards each connection to the Unix socket, then execs the client. Mount the `colony` binary read-only in the sandbox for this. Set `HTTPS_PROXY`/`HTTP_PROXY`/`https_proxy`/`http_proxy=http://127.0.0.1:3128` and `NO_PROXY=` (empty) for the client; drop the machine's own proxy variables from `passEnv` in filtered mode.
- The proxy (outside, in the guard) supports `CONNECT host:port` (HTTPS) and plain-HTTP forwarding, and allows **only** entries of the allowlist:
  - per client defaults, kept in code next to the adapters: Claude Code → `api.anthropic.com:443` (and only what a real run proves necessary; set `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` in the sandbox env so telemetry and update checks stay off); Antigravity → the Google endpoints a real run needs (find them with the log mode below; document the list);
  - the Colonyhall server's own host and port (from `guard.yaml` `server`), plain HTTP allowed only for it — so the MCP connection keeps working;
  - `guard.yaml` `sandbox_allow: ["host:port", …]` adds machine-wide entries.
  - Matching is exact host name (case-insensitive, no wildcards except an explicit leading `*.` in `sandbox_allow`) and exact port.
  - **After DNS resolution the proxy refuses** loopback, link-local, private (RFC 1918, CGNAT `100.64.0.0/10`, ULA `fc00::/7`), multicast and unspecified addresses — **except** the exact address of the Colonyhall server — so a name that resolves to the homelab's internal services (DNS rebinding, a hostile `/etc/hosts`) still gets nothing. IP literals are refused unless they are exactly the server's address.
  - Every refusal is logged once per host per run (`guard: sandbox network refused`, agent, host:port); never log request contents.
- `guard.yaml` `sandbox_network`: `filtered` (default), `log` (allow everything through the proxy but log each new `host:port` — used to discover what a client needs), `open` (today's behaviour: no `--unshare-net`, no proxy). Print the mode in the guard's startup line.
- If `--unshare-net` is not available (old `bwrap`, user namespaces off), `filtered` refuses sandboxed work runs with a clear reason in the chat (like a missing `bwrap` today), unless `sandbox_network: open`.
- Tests: proxy unit tests (allowed CONNECT, refused host, refused port, refused private/loopback/CGNAT IP after resolution with a fake resolver, IP literal refused, server address allowed, plain HTTP only to the server, `*.` rule, log mode); the bridge (bytes go both ways, closes cleanly, refuses when the socket is gone); with a real `bwrap` (skip if missing): inside the sandbox a direct TCP connect to a public IP fails, the same request through the proxy to an allowed local test server works, and a request to a not-allowed host gets 403.
- **Manual check, reported with results:** on this PC, one real sandboxed run with each client (`claude-code` and `antigravity`) in `log` mode, then in `filtered` mode with the discovered list; say which hosts each client needed and confirm both answer in the chat. If a client cannot work through an HTTP proxy at all, stop, keep that client on `open` by default, and explain in the report.

## Docs

Hungarian: `docs/01-terv.md` (the two sections: mark implemented, the decisions above), `docs/02-muszaki-specifikacio.md` (guard: worktree for sandboxed agents, the commit rules, the network design, `guard.yaml` keys), `guard.example.yaml` (`sandbox_network`, `sandbox_allow`, comments), `docs/05-homelab-telepites.md` (a short "Sandbox hálózat" note: modes and the rollback switch), `CLAUDE.md` schedule ("Később" line). Then the report `docs/handoff/18-sandbox-worktree-network-report.md` (Hungarian): commits, decisions where this prompt was open, the hosts each client needed, what is left out, the outputs of `make test`, `make lint`, `make gen-check`, `go test -race ./internal/guard/ ./internal/e2e/`, the manual check, and what the reviewer should look at first. `make test-pg` only if a store changed.
