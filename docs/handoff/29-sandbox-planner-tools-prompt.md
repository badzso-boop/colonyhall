# Handoff prompt 29: planner tools for sandboxed agents, and the "working…" indicator during the decide step

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/sandbox-planner-tools ../new-chat-worktrees/sandbox-planner-tools main
cd ../new-chat-worktrees/sandbox-planner-tools
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/29-sandbox-planner-tools-prompt.md)"
```

---

Branch: `feature/sandbox-planner-tools`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/sandbox-planner-tools` (created by the command above). This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never switch branches there, never touch `main`. Commit only on this branch; never merge, never push.

After each part: `gofmt`, `make test`, `make lint`, `make test-web` if the page changed; at the end `go test -race ./internal/guard/ ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it.

## 1. A sandboxed agent cannot read the planner (bug, round 28)

**Incident (2026-10-29):** Norbi created `@aszisztens` as the round 28 recipe says (homelab, sandbox, Antigravity flash-low, "munka" on) and asked it for today's tasks and calendar. The work run answered that `planner_read` and `calendar_read` are not available.

**Finding:**
- A sandboxed work run gets no Colonyhall MCP server at all. The only MCP server it can get is the guard's own `web_read` server:
  - `internal/guard/guard.go` `work`: `MCPConfigPath` is set only when `p.AllowWebRead`;
  - `internal/guard/sandbox.go` ~212–290: the bridge forwards `127.0.0.1:3129` to `webread.sock` and writes the `colony-web` MCP config for Claude Code and agy.
- Round 28 tested `planner_read` / `calendar_read` with an agent token directly against the server, never through the guard, so this was missed.

**Decision (Claude Code proposal, Norbi to confirm before merge):** use the same pattern as `web_read`.
- **The tools:** the guard runs a small **read-only MCP server outside the sandbox** with exactly two tools, `planner_read` and `calendar_read` (same names, inputs and output as the server's).
- **Forwarding:** for each call the guard calls the Colonyhall server's `/mcp` tool of the same name with **the agent's own short-lived token**, which stays in the guard. The token never enters the sandbox: no token in the sandbox's MCP config, environment or files.
- **Bridge:** reuse the bridge, either on its own socket and port (e.g. `planner.sock` → `127.0.0.1:3130`) or as extra tools on the existing `colony-web` server; explain the choice. Write the MCP config entry for both Claude Code and agy the way `colony-web` is written.
- **When:** only when the agent can read something.
  - Option a: always for sandboxed agents, since the server refuses agents the owner did not allow.
  - Option b: only when the server says the agent has planner or calendar access.

  Prefer option a unless it adds noise, and explain the choice.
- **Nothing else:** no other Colonyhall tool becomes available to sandboxed agents (no `send`, `ack`, `wiki_*` …). Sandboxed agents still answer through their final text only.
- **The prompt:** the work prompt mentions the two tools only when they are offered, in one line.
- **Network filter:** the sandbox network filter must not need a new hole. The tools are reached through the bridge socket, not over the network.
- **Tests:**
  - guard unit test: the run's MCP config offers `planner_read` and `calendar_read`, and it contains no token;
  - a test that calls the guard-side server and checks it forwards to the server with the agent's token and returns the server's text;
  - a test that an unknown tool name, or a write tool, is refused;
  - an e2e test with a sandboxed agent assigned to a host (scripted adapter, like the round 27 e2e tests): the work run's MCP config reaches the two tools, and a call returns the owner's tasks. It fails today because there is no such server.

## 2. No "working…" while a sandboxed agent decides (bug)

**Finding:** `withActivity` wraps only the work step (`internal/guard/guard.go` `work`) and the direct runs (`direct.go` `runDirectGroup`). The decide step in `handle` (`a.Adapter.Decide`) runs without it. On 2026-10-29 `@aszisztens`'s decide runs took 8–51 s, and the page showed nothing; when the decision is only a reply or a reaction, nothing is shown at all.

**Fix:**
- Send "working" for the decide step too: for each distinct (channel, thread) of the items in the batch, from the start of `Decide` until the actions are applied.
- If a work step follows for the same thread, the indicator must not flicker off between the two steps.
- A DM top-level item uses the same thread value the direct runs use, so the page matches it.
- **Tests:** a guard test with a scripted adapter whose `Decide` blocks until released: "working" is posted for the item's channel and thread before `Decide` returns and "idle" after; a batch with two threads posts both; decide followed by work does not post "idle" in between.

## Out of scope (say so in the report)

Other Colonyhall tools for sandboxed agents; the late wake after an approval (`task.md`, 2026-10-29) unless it is trivial on the way.

## Docs

Hungarian:
- `docs/02-muszaki-specifikacio.md`: the guard-side read-only planner MCP server and the bridge, and the decide-step activity.
- `docs/06-allapot-es-uzemeltetes.md`: in the morning summary recipe, a sandboxed agent works, with "munka" on.
- `task.md`: move the item to "Kész".

Then the report `docs/handoff/29-sandbox-planner-tools-report.md` (Hungarian) with:
- the commits and the choices left open (socket/port or shared server; when the tools are offered);
- the test outputs;
- a manual check on the dev stack with a sandboxed fake agent;
- what the reviewer should look at first (that the token never enters the sandbox).
