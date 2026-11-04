import { buildRows, visibleAgents, filterRows, filtersFromParams, filtersToParams, groupRows, noFilters, searchRows, type Actor, type Host } from "./directory";

const now = new Date("2026-10-31T10:00:00Z");
const actor = (id: string, handle: string, extra: Partial<Actor> = {}): Actor => ({ id, kind: "agent", handle, display_name: handle, role: "member", state: "active", created_at: "", ...extra });
const host = (id: string, name: string, seen: string | undefined, agents: Host["agents"]): Host => ({
  id, name, color: "blue", state: "approved", created_at: "", last_seen_at: seen, report: { roots: [], clients: [], rejected: [] } as unknown as Host["report"], agents,
});
const cfg = (agent_id: string, client: string, sandbox: boolean, model = "sonnet") =>
  ({ agent_id, handle: "", host_id: "", client, model, work_model: model, allow_work: true, permission_mode: "auto", mode: sandbox ? "compact" : "direct", root_dir: "", worktree_repo: "", sandbox, approvals: "none", max_session_tokens: 0, daily_token_limit: 0, grants: [] }) as Host["agents"][number];

const actors = [
  actor("a", "backend-claude", { description: "Go szerver" }),
  actor("b", "frontend-ag"),
  actor("c", "old-bot", { archived_at: "2026-10-30T00:00:00Z", state: "stopped" }),
  actor("d", "api-bot"),
  actor("e", "paused-bot", { state: "paused" }),
  { ...actor("h", "norbi"), kind: "human" } as Actor,
];
const hosts = [
  host("pc", "pc-wsl", "2026-10-31T09:59:30Z", [cfg("a", "claude-code", false), cfg("e", "claude-code", true)]),
  host("hl", "homelab", "2026-10-31T09:00:00Z", [cfg("b", "antigravity", true, "gemini")]),
];
const rows = buildRows(actors, hosts, new Set(["a"]), now);

test("rows: agents only, with their machine, access and state", () => {
  expect(rows.map((r) => [r.actor.handle, r.state, r.access ?? "-", r.host?.name ?? "-"])).toEqual([
    ["backend-claude", "working", "full", "pc-wsl"],
    ["frontend-ag", "offline", "sandbox", "homelab"],
    ["old-bot", "archived", "-", "-"],
    ["api-bot", "idle", "-", "-"],
    ["paused-bot", "paused", "sandbox", "pc-wsl"],
  ]);
});

test("search matches handle, description, machine, client, model and state", () => {
  expect(searchRows(rows, "szerver").map((r) => r.actor.handle)).toEqual(["backend-claude"]);
  expect(searchRows(rows, "homelab").map((r) => r.actor.handle)).toEqual(["frontend-ag"]);
  expect(searchRows(rows, "gemini").map((r) => r.actor.handle)).toEqual(["frontend-ag"]);
  expect(searchRows(rows, "bkc")[0]!.actor.handle).toBe("backend-claude");
  expect(searchRows(rows, "  ")).toHaveLength(5);
});

test("filters combine, archived ones only on request, and round-trip through the URL", () => {
  expect(filterRows(rows, noFilters).map((r) => r.actor.handle)).not.toContain("old-bot");
  const f = { ...noFilters, host: ["pc"], access: ["sandbox" as const] };
  expect(filterRows(rows, f).map((r) => r.actor.handle)).toEqual(["paused-bot"]);
  expect(filterRows(rows, { ...noFilters, archived: true, state: ["archived"] }).map((r) => r.actor.handle)).toEqual(["old-bot"]);
  const p = filtersToParams({ ...f, archived: true }, new URLSearchParams("q=x"));
  expect(p.toString()).toBe("q=x&host=pc&access=sandbox&archived=1");
  expect(filtersFromParams(p)).toEqual({ ...f, archived: true });
});

test("grouping by machine (API-key agents last) or state", () => {
  expect(groupRows(rows, "host").map((g) => [g.host?.name ?? "-", g.rows.length])).toEqual([["homelab", 1], ["pc-wsl", 2], ["-", 2]]);
  expect(groupRows(rows, "state").map((g) => g.key)).toEqual(["working", "idle", "paused", "offline", "archived"]);
});

test("a person who is not an admin sees only their own agents", () => {
  const list = [
    { id: "a", kind: "agent", owner_id: "kollega" },
    { id: "b", kind: "agent", owner_id: "norbi" },
    { id: "c", kind: "agent" },
    { id: "h", kind: "human" },
  ];
  expect(visibleAgents(list, { id: "kollega", role: "member" }).map((a) => a.id)).toEqual(["a", "h"]);
  expect(visibleAgents(list, { id: "norbi", role: "owner" }).map((a) => a.id)).toEqual(["a", "b", "c", "h"]);
  expect(visibleAgents(list, undefined)).toEqual([]);
});
