import { useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { Bot, LayoutGrid, Plus, Rows3, Search, Settings, ShieldAlert } from "lucide-react";
import { useHosts } from "@/features/hosts";
import { useSession } from "@/features/identity";
import { useLocalStorage } from "@/shared/hooks";
import { cn, hueVar, useShortcut } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Avatar, Badge, Button, EmptyState, Highlight, IconButton, Input, PageHeader, Select, Skeleton } from "@/shared/ui";
import { useActivity } from "../activity";
import { useAgentActors } from "../api";
import { buildRows, visibleAgents, filterRows, filtersFromParams, filtersToParams, groupRows, hostOnline, searchRows, type AgentRow, type AgentState, type Filters, type Grouping } from "../directory";

const stateTone: Record<AgentState, "success" | "neutral" | "warning" | "danger" | "info"> = {
  working: "success", idle: "neutral", offline: "warning", paused: "warning", stopped: "danger", archived: "neutral",
};

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cn("flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-meta", on ? "border-accent bg-accent-soft font-semibold text-fg" : "border-border text-fg-muted hover:bg-surface-sunken")}
    >
      {children}
    </button>
  );
}

/** Ügynökök (round 33, part 4): search, filters, grouping; a card opens the overview. */
export default function AgentsPage() {
  const { t } = useTranslation("agents");
  const [search, setSearch] = useSearchParams();
  const filters = filtersFromParams(search);
  const q = search.get("q") ?? "";
  const [grouping, setGrouping] = useLocalStorage<Grouping>("colony.agents.group", "host");
  const [view, setView] = useLocalStorage<"cards" | "table">("colony.agents.view", "cards");
  const actors = useAgentActors(filters.archived);
  const hosts = useHosts();
  const working = useActivity((s) => s.working);
  const session = useSession();
  const role = session.data?.actor.role;
  const admin = role === "owner" || role === "admin";
  const [, dispatch] = usePanes();
  const searchRef = useRef<HTMLInputElement>(null);
  useShortcut("/", "page", "agentSearch", () => searchRef.current?.focus());

  const now = new Date();
  const rows = useMemo(
    () =>
      buildRows(visibleAgents(actors.data ?? [], session.data?.actor), hosts.data ?? [], new Set(Object.keys(working).filter((k) => working[k]!.length > 0)), new Date()),
    [actors.data, hosts.data, working, session.data?.actor],
  );
  const shown = useMemo(() => searchRows(filterRows(rows, filters), q), [rows, filters, q]);
  const groups = groupRows(shown, q ? "none" : grouping);

  const setFilters = (f: Filters) => setSearch((prev) => filtersToParams(f, prev), { replace: true });
  const toggle = <K extends "host" | "client" | "access" | "state">(k: K, v: Filters[K][number]) => {
    const list = filters[k] as string[];
    setFilters({ ...filters, [k]: list.includes(v) ? list.filter((x) => x !== v) : [...list, v] });
  };
  const setQ = (v: string) =>
    setSearch(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (v) next.set("q", v);
        else next.delete("q");
        return next;
      },
      { replace: true },
    );
  const openOverview = (r: AgentRow) => dispatch({ type: "openWork", tab: { kind: "agent", agentId: r.actor.id, ref: "" } });
  const openSettings = (r: AgentRow) =>
    setSearch((prev) => {
      const next = new URLSearchParams(prev);
      next.set("agent", r.actor.handle);
      next.set("settings", "profil");
      return next;
    });

  const card = (r: AgentRow) => (
    <li key={r.actor.id} className="relative">
      <button
        type="button"
        onClick={() => openOverview(r)}
        className={cn("relative flex w-full flex-col gap-2 overflow-hidden rounded-lg border border-border bg-surface p-3 pl-4 text-left hover:border-border-strong", r.state === "archived" && "opacity-70")}
      >
        {r.host ? <span aria-hidden className="absolute inset-y-0 left-0 w-[3px]" style={{ background: hueVar(r.host.color) }} /> : null}
        <span className="flex items-center gap-2">
          <Avatar kind="agent" name={r.actor.display_name || r.actor.handle} emoji={r.actor.avatar_emoji} ringColor={hueVar(r.actor.effective_color)} />
          <span className="min-w-0 flex-1">
            <span className="block truncate font-semibold">
              <Highlight text={r.actor.display_name} query={q} />
            </span>
            <span className="block truncate font-mono text-meta text-fg-muted">
              @<Highlight text={r.actor.handle} query={q} />
            </span>
          </span>
        </span>
        <span className="flex flex-wrap items-center gap-1.5 text-meta">
          <Badge tone={stateTone[r.state]}>{t(`state.${r.state}`)}</Badge>
          {r.host?.report?.storage_paused && r.state !== "archived" ? <Badge tone="warning">{t("storagePaused")}</Badge> : null}
          {r.config ? (
            <span className="text-fg-muted">
              <Highlight text={`${r.config.client} · ${r.config.model || "–"}`} query={q} />
            </span>
          ) : (
            <span className="text-fg-muted">{t("create.apiKey")}</span>
          )}
          {r.access === "full" ? (
            <Badge tone="warning">
              <ShieldAlert aria-hidden className="size-3" /> {t("create.fullAccess")}
            </Badge>
          ) : r.access === "sandbox" ? (
            <Badge tone="info">{t("create.sandbox")}</Badge>
          ) : null}
        </span>
        {r.host ? (
          <span className="flex items-center gap-1 text-meta text-fg-subtle" title={r.host.name}>
            <span aria-hidden className="size-2 rounded-full" style={{ background: hueVar(r.host.color) }} />
            <Highlight text={r.host.name} query={q} />
          </span>
        ) : null}
        {r.actor.description ? (
          <span className="line-clamp-2 text-meta text-fg-muted">
            <Highlight text={r.actor.description} query={q} />
          </span>
        ) : null}
      </button>
      <IconButton
        size="sm"
        label={t("directory.settings", { handle: r.actor.handle })}
        icon={<Settings className="size-3.5" />}
        onClick={() => openSettings(r)}
        className="absolute top-2 right-2"
      />
    </li>
  );

  const table = (list: AgentRow[]) => (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full min-w-[44rem] text-left text-ui">
        <thead className="bg-surface-sunken text-meta text-fg-muted">
          <tr>
            {(["agent", "state", "host", "client", "access"] as const).map((c) => (
              <th key={c} scope="col" className="px-3 py-2 font-semibold">
                {t(`directory.cols.${c}`)}
              </th>
            ))}
            <th scope="col" className="px-3 py-2">
              <span className="sr-only">{t("directory.cols.actions")}</span>
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {list.map((r) => (
            <tr key={r.actor.id} className={cn("hover:bg-surface-sunken", r.state === "archived" && "opacity-70")}>
              <td className="px-3 py-1.5" style={{ boxShadow: r.host ? `inset 3px 0 0 ${hueVar(r.host.color)}` : undefined }}>
                <button type="button" onClick={() => openOverview(r)} className="flex items-center gap-2 text-left">
                  <Avatar kind="agent" size="sm" name={r.actor.display_name || r.actor.handle} emoji={r.actor.avatar_emoji} ringColor={hueVar(r.actor.effective_color)} />
                  <span className="font-mono">
                    @<Highlight text={r.actor.handle} query={q} />
                  </span>
                </button>
              </td>
              <td className="px-3 py-1.5">
                <Badge tone={stateTone[r.state]}>{t(`state.${r.state}`)}</Badge>
              </td>
              <td className="px-3 py-1.5">{r.host?.name ?? "–"}</td>
              <td className="px-3 py-1.5">{r.config ? `${r.config.client} · ${r.config.model || "–"}` : t("create.apiKey")}</td>
              <td className="px-3 py-1.5">{r.access ? t(r.access === "full" ? "create.fullAccess" : "create.sandbox") : "–"}</td>
              <td className="px-2 py-1">
                <IconButton size="sm" label={t("directory.settings", { handle: r.actor.handle })} icon={<Settings className="size-3.5" />} onClick={() => openSettings(r)} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<Bot className="size-5" />}
        title={t("directory.title")}
        actions={
          admin ? (
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="size-4" />}
              onClick={() => setSearch((prev) => new URLSearchParams({ ...Object.fromEntries(prev), new: "1" }))}
            >
              {t("directory.new")}
            </Button>
          ) : null
        }
      />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <div className="flex flex-wrap items-center gap-2">
          <label className="relative min-w-56 flex-1">
            <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
            <Input ref={searchRef} type="search" aria-label={t("directory.search")} placeholder={t("directory.search")} value={q} onChange={(e) => setQ(e.target.value)} className="pl-8" />
          </label>
          <Select aria-label={t("directory.group")} value={grouping} onChange={(e) => setGrouping(e.target.value as Grouping)} className="w-44">
            {(["host", "state", "none"] as const).map((g) => (
              <option key={g} value={g}>
                {t(`directory.groups.${g}`)}
              </option>
            ))}
          </Select>
          <div role="radiogroup" aria-label={t("directory.view")} className="flex rounded-md border border-border p-0.5">
            {(["cards", "table"] as const).map((v) => (
              <IconButton
                key={v}
                size="sm"
                role="radio"
                aria-checked={view === v}
                label={t(`directory.views.${v}`)}
                icon={v === "cards" ? <LayoutGrid className="size-4" /> : <Rows3 className="size-4" />}
                onClick={() => setView(v)}
                className={view === v ? "bg-accent-soft text-fg" : undefined}
              />
            ))}
          </div>
        </div>
        <div role="group" aria-label={t("directory.filters")} className="flex flex-wrap items-center gap-1.5">
          {(hosts.data ?? []).map((h) => (
            <Chip key={h.id} on={filters.host.includes(h.id)} onClick={() => toggle("host", h.id)}>
              <span aria-hidden className="size-2 rounded-full" style={{ background: hueVar(h.color) }} />
              {h.name}
            </Chip>
          ))}
          {(["claude-code", "antigravity"] as const).map((c) => (
            <Chip key={c} on={filters.client.includes(c)} onClick={() => toggle("client", c)}>
              {c}
            </Chip>
          ))}
          {(["sandbox", "full"] as const).map((a) => (
            <Chip key={a} on={filters.access.includes(a)} onClick={() => toggle("access", a)}>
              {t(a === "full" ? "create.fullAccess" : "create.sandbox")}
            </Chip>
          ))}
          {(["working", "idle", "offline", "paused", "stopped"] as const).map((s) => (
            <Chip key={s} on={filters.state.includes(s)} onClick={() => toggle("state", s)}>
              {t(`state.${s}`)}
            </Chip>
          ))}
          <Chip on={filters.archived} onClick={() => setFilters({ ...filters, archived: !filters.archived })}>
            {t("state.archived")}
          </Chip>
        </div>
        {actors.isPending ? <Skeleton className="h-24 w-full" /> : null}
        {actors.data && shown.length === 0 ? <EmptyState icon={<Bot className="size-8" />} title={q ? t("directory.noMatch") : t("directory.empty")} /> : null}
        {groups.map((g) => (
          <section key={g.key} aria-label={g.host?.name ?? (grouping === "state" ? t(`state.${g.key}`) : t("directory.noHost"))}>
            {groups.length > 1 || grouping !== "none" ? (
              <h2 className="mb-2 flex items-center gap-2 text-meta font-semibold tracking-wide text-fg-subtle uppercase">
                {g.host ? <span aria-hidden className="size-2.5 rounded-full" style={{ background: hueVar(g.host.color) }} /> : null}
                {g.host?.name ?? (grouping === "state" && !q ? t(`state.${g.key}`) : g.key === "all" ? t("directory.all") : t("directory.noHost"))}
                {g.host ? <span className="font-normal normal-case">· {hostOnline(g.host, now) ? t("create.online") : t("create.offline")}</span> : null}
                <span className="font-normal">· {t("directory.count", { count: g.rows.length })}</span>
              </h2>
            ) : null}
            {view === "cards" ? <ul className="grid gap-2 sm:grid-cols-2 xl:grid-cols-3">{g.rows.map(card)}</ul> : table(g.rows)}
          </section>
        ))}
      </div>
    </div>
  );
}
