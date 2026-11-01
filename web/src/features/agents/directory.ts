import type { Schemas } from "@/shared/api";
import { fuzzyScore } from "@/shared/lib";

// The agent directory (docs/handoff/33, part 4): one row per agent with its
// machine configuration, searched, filtered and grouped on the client.

export type Actor = Schemas["Actor"];
export type Host = Schemas["Host"];
export type HostAgent = Schemas["HostAgent"];
export type AgentState = "working" | "idle" | "offline" | "paused" | "stopped" | "archived";
export type Access = "sandbox" | "full";

export type AgentRow = {
  actor: Actor;
  host?: Host;
  config?: HostAgent;
  state: AgentState;
  access?: Access;
};

/** A machine is online when its guard was seen within 90 s. */
export function hostOnline(h: Pick<Host, "last_seen_at" | "state">, now: Date): boolean {
  return h.state === "approved" && Boolean(h.last_seen_at) && now.getTime() - new Date(h.last_seen_at!).getTime() < 90_000;
}

export function agentState(actor: Actor, host: Host | undefined, working: boolean, now: Date): AgentState {
  if (actor.archived_at) return "archived";
  if (actor.state === "stopped") return "stopped";
  if (actor.state === "paused") return "paused";
  if (working) return "working";
  if (host && !hostOnline(host, now)) return "offline";
  return "idle";
}

export function buildRows(actors: Actor[], hosts: Host[], working: Set<string>, now: Date): AgentRow[] {
  const byAgent = new Map<string, { host: Host; config: HostAgent }>();
  for (const h of hosts) for (const c of h.agents) byAgent.set(c.agent_id, { host: h, config: c });
  return actors
    .filter((a) => a.kind === "agent")
    .map((actor) => {
      const m = byAgent.get(actor.id);
      return {
        actor,
        host: m?.host,
        config: m?.config,
        state: agentState(actor, m?.host, working.has(actor.id), now),
        access: m ? (m.config.sandbox ? "sandbox" : "full") : undefined,
      };
    });
}

/** The fields the search looks at, for highlighting too. */
export function searchFields(r: AgentRow): string[] {
  return [r.actor.handle, r.actor.display_name, r.actor.description ?? "", r.host?.name ?? "", r.config?.client ?? "", r.config?.model ?? "", r.state];
}

/** Rows matching the query (fuzzy), best first; all rows for an empty query. */
export function searchRows(rows: AgentRow[], query: string): AgentRow[] {
  const q = query.trim().toLowerCase();
  if (!q) return rows;
  return rows
    .map((r) => ({ r, score: Math.max(...searchFields(r).map((f) => fuzzyScore(f, q))) }))
    .filter((x) => x.score > 0)
    .sort((a, b) => b.score - a.score || a.r.actor.handle.localeCompare(b.r.actor.handle))
    .map((x) => x.r);
}

export type Filters = { host: string[]; client: string[]; access: Access[]; state: AgentState[]; archived: boolean };
export const noFilters: Filters = { host: [], client: [], access: [], state: [], archived: false };

export function filterRows(rows: AgentRow[], f: Filters): AgentRow[] {
  return rows.filter(
    (r) =>
      (f.archived ? true : r.state !== "archived") &&
      (!f.host.length || (r.host && f.host.includes(r.host.id))) &&
      (!f.client.length || (r.config && f.client.includes(r.config.client))) &&
      (!f.access.length || (r.access && f.access.includes(r.access))) &&
      (!f.state.length || f.state.includes(r.state)),
  );
}

/** The filters as URL search params, and back (kept in the URL). */
export function filtersToParams(f: Filters, params: URLSearchParams): URLSearchParams {
  const out = new URLSearchParams(params);
  for (const k of ["host", "client", "access", "state"] as const) {
    out.delete(k);
    if (f[k].length) out.set(k, f[k].join(","));
  }
  out.delete("archived");
  if (f.archived) out.set("archived", "1");
  return out;
}

export function filtersFromParams(p: URLSearchParams): Filters {
  const list = <T extends string>(k: string) => (p.get(k)?.split(",").filter(Boolean) ?? []) as T[];
  return { host: list("host"), client: list("client"), access: list<Access>("access"), state: list<AgentState>("state"), archived: p.get("archived") === "1" };
}

export type Grouping = "host" | "state" | "none";
export type Group = { key: string; rows: AgentRow[]; host?: Host };

const stateOrder: AgentState[] = ["working", "idle", "paused", "offline", "stopped", "archived"];

export function groupRows(rows: AgentRow[], by: Grouping): Group[] {
  if (by === "none") return [{ key: "all", rows }];
  const groups = new Map<string, Group>();
  for (const r of rows) {
    const key = by === "host" ? (r.host?.id ?? "") : r.state;
    const g = groups.get(key) ?? { key, rows: [], host: by === "host" ? r.host : undefined };
    g.rows.push(r);
    groups.set(key, g);
  }
  const out = [...groups.values()];
  if (by === "state") return out.sort((a, b) => stateOrder.indexOf(a.key as AgentState) - stateOrder.indexOf(b.key as AgentState));
  // Machines by name; API-key agents (no machine) last.
  return out.sort((a, b) => (!a.host ? 1 : !b.host ? -1 : a.host.name.localeCompare(b.host.name)));
}
