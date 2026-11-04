import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Activity, ArrowDown, ArrowUp, CircleAlert, CircleCheck, CircleDashed, Globe } from "lucide-react";
import { useActors } from "@/features/identity";
import { useChannels } from "@/features/conversation";
import { useHosts } from "@/features/hosts";
import type { Schemas } from "@/shared/api";
import { localeOf } from "@/shared/i18n";
import { cn } from "@/shared/lib";
import { Button, EmptyState, Input, Select, Skeleton, Tab, TabList, Tabs } from "@/shared/ui";
import { useCurrency, useRunPages, useUsage, type RunFilters } from "../api";
import { AdminPage } from "../components/AdminPage";
import { LimitRing, seriesColor, Sparkline, StackedBars, type Series } from "../components/charts";
import { RunSheet } from "../components/RunSheet";
import {
  agentRows,
  assignHues,
  budapestDay,
  dayList,
  formatDuration,
  formatHuf,
  formatUsd,
  OTHER_KEY,
  periodDays,
  periodTiles,
  shortTokens,
  sortRows,
  stackDays,
  toHuf,
  type AgentRow,
  type Period,
  type SortDir,
} from "../model";

type GroupBy = "agent" | "host" | "model";
const periods = ["today", "7", "30", "90", "custom"] as const;

function Tile({ label, value, note }: { label: string; value: string; note?: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-0.5 rounded-lg border border-border bg-surface p-3">
      <span className="text-meta text-fg-subtle">{label}</span>
      <span className="tabular truncate text-title font-semibold">{value}</span>
      {note ? <span className="text-meta text-fg-muted">{note}</span> : null}
    </div>
  );
}

function Card({ title, children, actions }: { title: string; children: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-lg border border-border bg-surface p-3">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="mr-auto font-semibold">{title}</h2>
        {actions}
      </div>
      {children}
    </section>
  );
}

/** Futások (/admin/runs): the period, the tiles and limits, the charts, the agents, the budgets and the run list. */
export default function RunsPage() {
  const { t } = useTranslation("admin");
  return (
    <AdminPage icon={<Activity className="size-5" />} title={t("runs.title")}>
      <RunsBody />
    </AdminPage>
  );
}

function RunsBody() {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const today = budapestDay(new Date());
  const [period, setPeriod] = useState<Period>({ kind: "7" });
  const [groupBy, setGroupBy] = useState<GroupBy>("agent");
  // Személy (round 38): the agents of one person, for the tiles, the chart and the list.
  const [owner, setOwner] = useState("");
  const range = periodDays(period, today);
  const days = useMemo(() => dayList(range.from, range.to), [range.from, range.to]);
  const usage = useUsage(range.fetchDays, groupBy, owner);
  const byAgent = useUsage(range.fetchDays, "agent", owner);
  const actors = useActors();
  const people = [...(actors.data?.values() ?? [])].filter((a) => a.kind === "human" && !a.archived_at);
  const hosts = useHosts(true, 60_000);
  const currency = useCurrency();
  const rate = currency.data?.usd_huf;
  const num = (n: number) => shortTokens(n, locale);

  const actorName = (id: string) => {
    const a = actors.data?.get(id);
    return a ? `@${a.handle}` : t("runs.unknownAgent");
  };
  const seriesName = (key: string) => {
    if (key === OTHER_KEY) return t("runs.other");
    if (groupBy === "agent") return actorName(key);
    if (groupBy === "host") return hosts.data?.find((h) => h.id === key)?.name ?? (key ? key.slice(0, 8) : t("runs.noMachine"));
    return key || t("runs.noModel");
  };

  const groups = usage.data?.groups ?? [];
  const agentGroups = byAgent.data?.groups ?? [];
  const tiles = periodTiles(groups, range.from, range.to);
  const stack = stackDays(groups, days, (g) => g.total_tokens);
  const hues = assignHues(
    stack.keys.filter((k) => k !== OTHER_KEY),
    (k) => (groupBy === "agent" ? actors.data?.get(k)?.effective_color : groupBy === "host" ? hosts.data?.find((h) => h.id === k)?.color : undefined),
  );
  const series: Series[] = stack.keys.map((key) => ({ key, label: seriesName(key), color: seriesColor(key, hues.get(key)) }));
  const failures = stackDays(
    groups.flatMap((g) => [
      { ...g, key: "ok", run_count: g.run_count - g.error_count },
      { ...g, key: "failed", run_count: g.error_count },
    ]),
    days,
    (g) => g.run_count,
  );
  const runSeries: Series[] = [
    { key: "ok", label: t("runs.okRuns"), color: "var(--color-info)" },
    { key: "failed", label: t("runs.failedRuns"), color: "var(--color-danger)" },
  ];

  const totals = usage.data?.totals;
  const custom = period.kind === "custom" && range.to !== today;
  const huf = toHuf(tiles.usd, rate);
  const costValue = tiles.hasUsd ? `~${formatUsd(tiles.usd, locale)}${huf != null ? ` · ~${formatHuf(huf, locale)}` : ""}` : t("runs.noPrice");
  const costNote = [
    totals && totals.subscription_usd > 0 ? t("runs.subscription") : null,
    totals && totals.unpriced_runs > 0 ? t("runs.unpriced", { count: totals.unpriced_runs }) : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto bg-surface-sunken p-3 sm:p-4">
      <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("runs.period")}>
        {periods.map((p) => (
          <Button
            key={p}
            size="sm"
            variant={period.kind === p ? "primary" : "secondary"}
            onClick={() => setPeriod(p === "custom" ? { kind: "custom", from: range.from, to: range.to } : { kind: p })}
          >
            {t(`runs.periods.${p}`)}
          </Button>
        ))}
        {period.kind === "custom" ? (
          <span className="flex flex-wrap items-center gap-1">
            <Input type="date" aria-label={t("runs.from")} value={period.from} max={today} onChange={(e) => e.target.value && setPeriod({ ...period, from: e.target.value })} className="w-36" />
            <Input type="date" aria-label={t("runs.to")} value={period.to} max={today} onChange={(e) => e.target.value && setPeriod({ ...period, to: e.target.value })} className="w-36" />
          </span>
        ) : null}
        <Select aria-label={t("runs.filter.owner")} value={owner} onChange={(e) => setOwner(e.target.value)} className="w-auto">
          <option value="">{t("runs.filter.ownerAll")}</option>
          {people.map((p) => (
            <option key={p.id} value={p.id}>
              {p.display_name} (@{p.handle})
            </option>
          ))}
        </Select>
      </div>

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        {usage.isPending ? (
          [0, 1, 2, 3, 4].map((i) => <Skeleton key={i} className="h-20 w-full" />)
        ) : (
          <>
            <Tile label={t("runs.tiles.tokens")} value={num(tiles.tokens)} />
            <Tile label={t("runs.tiles.runs")} value={tiles.runs.toLocaleString(locale)} />
            <Tile label={t("runs.tiles.failures")} value={tiles.failures.toLocaleString(locale)} />
            <Tile label={t("runs.tiles.duration")} value={!custom && totals?.run_count ? formatDuration(totals.avg_duration_ms) : "–"} />
            <div className="col-span-2 lg:col-span-1">
              <Tile label={t("runs.tiles.cost")} value={costValue} note={costNote || undefined} />
            </div>
          </>
        )}
      </div>

      <MachineLimits />

      <div className="grid min-w-0 gap-3 xl:grid-cols-2">
        <Card
          title={t("runs.tokensChart")}
          actions={
            <Tabs value={groupBy} onValueChange={(v) => setGroupBy(v as GroupBy)}>
              <TabList label={t("runs.groupBy")}>
                {(["agent", "host", "model"] as const).map((g) => (
                  <Tab key={g} value={g}>
                    {t(`runs.group.${g}`)}
                  </Tab>
                ))}
              </TabList>
            </Tabs>
          }
        >
          {tiles.tokens > 0 ? <StackedBars days={stack.days} series={series} label={t("runs.tokensChart")} format={num} /> : <EmptyState title={t("runs.noUsage")} />}
        </Card>
        <Card title={t("runs.runsChart")}>
          {tiles.runs > 0 ? <StackedBars days={failures.days} series={runSeries} label={t("runs.runsChart")} format={(n) => Math.round(n).toLocaleString(locale)} /> : <EmptyState title={t("runs.noUsage")} />}
        </Card>
      </div>

      <AgentsTable rows={agentRows(agentGroups, days)} name={actorName} rate={rate} />
      <Budgets />
      <RunList from={range.from} to={range.to} rate={rate} owner={owner} />
    </div>
  );
}

function MachineLimits() {
  const { t, i18n } = useTranslation("admin");
  const hosts = useHosts(true, 60_000);
  const withLimits = (hosts.data ?? []).filter((h) => h.report.claude_limits?.five_hour || h.report.agy_usage?.length);
  if (!withLimits.length) return null;
  const when = (at: string | undefined) => (at ? new Date(at).toLocaleString(localeOf(i18n.language), { weekday: "short", hour: "2-digit", minute: "2-digit" }) : "–");
  return (
    <Card title={t("runs.limits")}>
      <ul className="flex flex-wrap gap-x-8 gap-y-3">
        {withLimits.map((h) => {
          const l = h.report.claude_limits;
          const five = Math.round((l?.five_hour?.utilization ?? 0) * 100);
          const week = Math.round((l?.seven_day?.utilization ?? 0) * 100);
          const used = (remaining: number) => Math.round((1 - remaining) * 100);
          return (
            <li key={h.id} className="flex flex-wrap items-center gap-3">
              <span className="font-medium">{h.name}</span>
              {l?.five_hour ? (
                <>
                  <span className="text-meta text-fg-subtle">Claude</span>
                  <LimitRing pct={five} label={t("runs.fiveHour")} title={t("runs.ringTitle", { name: `${h.name} Claude`, window: t("runs.fiveHour"), pct: five, reset: when(l.five_hour?.resets_at) })} />
                  <LimitRing pct={week} label={t("runs.week")} title={t("runs.ringTitle", { name: `${h.name} Claude`, window: t("runs.week"), pct: week, reset: when(l.seven_day?.resets_at) })} />
                </>
              ) : null}
              {(h.report.agy_usage ?? []).map((g) => (
                <span key={g.name} className="flex flex-wrap items-center gap-3">
                  <span className="text-meta text-fg-subtle" title={g.models ?? undefined}>
                    {g.name}
                  </span>
                  {g.five_hour ? <LimitRing pct={used(g.five_hour.remaining)} label={t("runs.fiveHour")} title={t("runs.ringTitle", { name: g.name, window: t("runs.fiveHour"), pct: used(g.five_hour.remaining), reset: when(g.five_hour.resets_at) })} /> : null}
                  {g.weekly ? <LimitRing pct={used(g.weekly.remaining)} label={t("runs.week")} title={t("runs.ringTitle", { name: g.name, window: t("runs.week"), pct: used(g.weekly.remaining), reset: when(g.weekly.resets_at) })} /> : null}
                </span>
              ))}
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

type SortKey = "name" | "runs" | "input" | "output" | "cache" | "usd" | "failures" | "lastDay";

function AgentsTable({ rows, name, rate }: { rows: AgentRow[]; name: (id: string) => string; rate?: number }) {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>({ key: "runs", dir: "desc" });
  const value = (r: AgentRow): string | number | null => (sort.key === "name" ? name(r.id) : r[sort.key]);
  const sorted = sortRows(rows, value, sort.dir);
  const head = (key: SortKey, right = true) => (
    <th scope="col" aria-sort={sort.key === key ? (sort.dir === "asc" ? "ascending" : "descending") : "none"} className={cn("px-2 py-1.5 font-medium whitespace-nowrap", right && "text-right")}>
      <button type="button" onClick={() => setSort({ key, dir: sort.key === key && sort.dir === "desc" ? "asc" : "desc" })} className="inline-flex items-center gap-1 hover:text-fg">
        {t(`runs.table.${key}`)}
        {sort.key === key ? sort.dir === "asc" ? <ArrowUp aria-hidden className="size-3" /> : <ArrowDown aria-hidden className="size-3" /> : null}
      </button>
    </th>
  );
  return (
    <Card title={t("runs.agents")}>
      {rows.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-ui">
            <thead className="border-b border-border text-meta text-fg-subtle">
              <tr>
                {head("name", false)}
                {head("runs")}
                {head("input")}
                {head("output")}
                {head("cache")}
                {head("usd")}
                {head("failures")}
                {head("lastDay")}
                <th scope="col" className="px-2 py-1.5 text-left font-medium">
                  {t("runs.table.trend")}
                </th>
              </tr>
            </thead>
            <tbody className="tabular">
              {sorted.map((r) => {
                const huf = r.usd != null ? toHuf(r.usd, rate) : null;
                return (
                  <tr key={r.id} className="border-b border-border/60 last:border-0">
                    <th scope="row" className="px-2 py-1.5 text-left font-medium whitespace-nowrap">
                      {name(r.id)}
                    </th>
                    <td className="px-2 text-right">{r.runs.toLocaleString(locale)}</td>
                    <td className="px-2 text-right">{shortTokens(r.input, locale)}</td>
                    <td className="px-2 text-right">{shortTokens(r.output, locale)}</td>
                    <td className="px-2 text-right">{shortTokens(r.cache, locale)}</td>
                    <td className="px-2 text-right whitespace-nowrap" title={huf != null ? `~${formatHuf(huf, locale)}` : undefined}>
                      {r.usd != null ? `~${formatUsd(r.usd, locale)}` : t("runs.noPrice")}
                    </td>
                    <td className={cn("px-2 text-right", r.failures > 0 && "font-semibold text-danger")}>{r.failures.toLocaleString(locale)}</td>
                    <td className="px-2 text-right whitespace-nowrap">{r.lastDay ?? "–"}</td>
                    <td className="px-2">
                      <Sparkline values={r.spark} label={name(r.id)} />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        <EmptyState title={t("runs.noUsage")} />
      )}
    </Card>
  );
}

function Budgets() {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const hosts = useHosts(true, 60_000);
  const usage = useUsage(1, "agent");
  const today = budapestDay(new Date());
  const used = new Map((usage.data?.groups ?? []).filter((g) => g.day === today).map((g) => [g.key, g.total_tokens]));
  const agents = (hosts.data ?? []).flatMap((h) => h.agents).filter((a) => (a.daily_token_limit ?? 0) > 0);
  if (!agents.length) return null;
  return (
    <Card title={t("runs.budgets")}>
      <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {agents.map((a) => {
          const limit = a.daily_token_limit ?? 0;
          const tokens = used.get(a.agent_id) ?? 0;
          const pct = Math.min(100, (tokens / limit) * 100);
          return (
            <li key={a.agent_id} className="flex flex-col gap-1">
              <div className="flex items-baseline gap-2 text-ui">
                <Link to={`/agents?agent=${encodeURIComponent(a.handle)}&settings=keretek`} className="min-w-0 truncate font-medium hover:underline">
                  @{a.handle}
                </Link>
                <span className="tabular ml-auto text-meta text-fg-muted">
                  {shortTokens(tokens, locale)} / {shortTokens(limit, locale)}
                </span>
              </div>
              <div role="progressbar" aria-label={`@${a.handle}`} aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100} className="h-1.5 overflow-hidden rounded-full bg-border">
                <div className={cn("h-full rounded-full", pct >= 80 ? "bg-warning" : "bg-info")} style={{ width: `${pct}%` }} />
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

function StatusIcon({ status }: { status: Schemas["Run"]["status"] }) {
  const { t } = useTranslation("admin");
  const label = t(`runs.status.${status}`);
  if (status === "error") return <CircleAlert role="img" aria-label={label} className="size-4 shrink-0 text-danger" />;
  if (status === "halted") return <CircleDashed role="img" aria-label={label} className="size-4 shrink-0 text-warning" />;
  return <CircleCheck role="img" aria-label={label} className="size-4 shrink-0 text-success" />;
}

function RunList({ from, to, rate, owner }: { from: string; to: string; rate?: number; owner: string }) {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const actors = useActors();
  const hosts = useHosts(true, 60_000);
  const channels = useChannels();
  const [filters, setFilters] = useState<Pick<RunFilters, "agent_id" | "host_id" | "channel_id" | "status">>({});
  const query = useMemo<RunFilters>(
    () => ({
      ...filters,
      ...(owner ? { owner_id: owner } : {}),
      from: new Date(`${from}T00:00:00`).toISOString(),
      to: new Date(new Date(`${to}T00:00:00`).getTime() + 86_400_000).toISOString(),
    }),
    [filters, from, to, owner],
  );
  const pages = useRunPages(query);
  const runs = pages.data?.pages.flatMap((p) => p.runs) ?? [];
  const [open, setOpen] = useState<Schemas["Run"] | null>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = pages;
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasNextPage || typeof IntersectionObserver === "undefined") return;
    const seen = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting) && !isFetchingNextPage) void fetchNextPage();
    });
    seen.observe(el);
    return () => seen.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);
  const set = (patch: Partial<typeof filters>) => setFilters((f) => ({ ...f, ...patch }));
  const agents = [...(actors.data?.values() ?? [])].filter((a) => a.kind === "agent" && (!owner || a.owner_id === owner));
  const channelName = (id?: string) => (id ? channels.data?.find((c) => c.id === id)?.name : undefined);
  const nameOf = (id: string) => {
    const a = actors.data?.get(id);
    return a ? `@${a.handle}` : t("runs.unknownAgent");
  };
  return (
    <Card title={t("runs.list")}>
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <Select aria-label={t("runs.filter.agent")} value={filters.agent_id ?? ""} onChange={(e) => set({ agent_id: e.target.value || undefined })}>
          <option value="">{t("runs.filter.agentAll")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              @{a.handle}
            </option>
          ))}
        </Select>
        <Select aria-label={t("runs.filter.host")} value={filters.host_id ?? ""} onChange={(e) => set({ host_id: e.target.value || undefined })}>
          <option value="">{t("runs.filter.hostAll")}</option>
          {(hosts.data ?? []).map((h) => (
            <option key={h.id} value={h.id}>
              {h.name}
            </option>
          ))}
        </Select>
        <Select aria-label={t("runs.filter.channel")} value={filters.channel_id ?? ""} onChange={(e) => set({ channel_id: e.target.value || undefined })}>
          <option value="">{t("runs.filter.channelAll")}</option>
          {(channels.data ?? [])
            .filter((c) => c.kind !== "dm")
            .map((c) => (
              <option key={c.id} value={c.id}>
                #{c.name}
              </option>
            ))}
        </Select>
        <Select aria-label={t("runs.filter.status")} value={filters.status ?? ""} onChange={(e) => set({ status: (e.target.value || undefined) as RunFilters["status"] })}>
          <option value="">{t("runs.filter.statusAll")}</option>
          {(["ok", "error", "halted"] as const).map((s) => (
            <option key={s} value={s}>
              {t(`runs.status.${s}`)}
            </option>
          ))}
        </Select>
      </div>
      {pages.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : pages.isError ? (
        <p className="text-ui text-danger">{pages.error.message}</p>
      ) : runs.length ? (
        <ul className="flex flex-col">
          {runs.map((r) => {
            const tokens = r.input_tokens + r.output_tokens;
            const name = channelName(r.channel_id);
            const threadLink = r.channel_id ? `/c/${r.channel_id}${r.thread_root_id ? `?thread=${r.thread_root_id}` : ""}` : null;
            return (
              <li key={r.id} className="border-b border-border/60 last:border-0">
                <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 py-1.5 text-ui hover:bg-surface-sunken">
                  <button type="button" onClick={() => setOpen(r)} className="flex min-w-0 flex-1 basis-56 items-center gap-2 text-left">
                    <StatusIcon status={r.status} />
                    <span className="truncate font-medium">{nameOf(r.agent_id)}</span>
                    <span className="text-meta text-fg-subtle">{t(`runs.phase.${r.phase}`)}</span>
                  </button>
                  {threadLink ? (
                    <Link to={threadLink} className="max-w-40 truncate text-meta text-fg-muted hover:underline">
                      {name ? `#${name}` : t("runs.dm")}
                    </Link>
                  ) : null}
                  <span className="tabular flex flex-wrap items-center gap-x-3 text-meta text-fg-muted">
                    <span>{new Date(r.created_at).toLocaleString(locale, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}</span>
                    <span>{formatDuration(r.duration_ms)}</span>
                    <span>{shortTokens(tokens, locale)}</span>
                    <span>{r.estimated_usd != null ? `~${formatUsd(r.estimated_usd, locale)}` : t("runs.noPrice")}</span>
                    {r.web_reads?.length ? (
                      <span className="inline-flex items-center gap-0.5" title={t("runs.sheet.web", { count: r.web_reads.length })}>
                        <Globe aria-hidden className="size-3" />
                        {r.web_reads.length}
                      </span>
                    ) : null}
                  </span>
                </div>
              </li>
            );
          })}
        </ul>
      ) : (
        <EmptyState title={t("runs.noRuns")} />
      )}
      {hasNextPage ? (
        <div ref={sentinel} className="flex justify-center">
          <Button size="sm" variant="ghost" loading={isFetchingNextPage} onClick={() => void fetchNextPage()}>
            {t("more")}
          </Button>
        </div>
      ) : null}
      {open ? <RunSheet run={open} agent={nameOf(open.agent_id)} channel={channelName(open.channel_id)} rate={rate} onClose={() => setOpen(null)} /> : null}
    </Card>
  );
}
