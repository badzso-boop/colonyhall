import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDown, ArrowUp, Copy, KeyRound, Search, Unlock } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { useNow, useWidth } from "@/shared/hooks";
import { cn, fullTime, relativeTime } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Avatar, Badge, Button, Dialog, EmptyState, IconButton, Input, PageHeader, Select, Skeleton, toast } from "@/shared/ui";
import { useLeases, useReleaseLease } from "../api";
import { Soon, countdown, filterLeases, leaseState, remaining, sortLeases, type Lease, type SortKey } from "../model";

const tone = { active: "success", expired: "danger", released: "neutral" } as const;
const columns: (SortKey | "left" | "action")[] = ["resource", "holder", "note", "acquired", "expires", "left", "state", "action"];

/** Foglalások (round 32, part 8): a live, sortable table; cards on a phone. */
export default function LeasesPage() {
  const { t, i18n } = useTranslation("leases");
  const locale = localeOf(i18n.language);
  const now = useNow(1000);
  const narrow = useWidth() === "narrow";
  const actors = useActors();
  const session = useSession();
  const human = session.data?.actor.kind === "human";
  const [, dispatch] = usePanes();
  const [history, setHistory] = useState(false);
  const [holder, setHolder] = useState("");
  const [text, setText] = useState("");
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "expires", desc: false });
  const [confirm, setConfirm] = useState<Lease | null>(null);
  const leases = useLeases(history);
  const release = useReleaseLease();

  // A lease that appears after the first load flashes once.
  const known = useRef<Set<string> | null>(null);
  const [flash, setFlash] = useState<Set<string>>(new Set());
  useEffect(() => {
    if (!leases.data) return;
    const ids = new Set(leases.data.map((l) => l.id));
    const fresh = known.current ? [...ids].filter((id) => !known.current!.has(id)) : [];
    known.current = ids;
    if (!fresh.length) return;
    const show = window.setTimeout(() => setFlash(new Set(fresh)), 0);
    const hide = window.setTimeout(() => setFlash(new Set()), 1500);
    return () => {
      window.clearTimeout(show);
      window.clearTimeout(hide);
    };
  }, [leases.data]);

  const holders = useMemo(() => [...new Map((leases.data ?? []).map((l) => [l.holder_id, l.holder_handle])).entries()].sort((a, b) => a[1].localeCompare(b[1])), [leases.data]);
  const rows = useMemo(() => sortLeases(filterLeases(leases.data ?? [], text, holder), sort.key, sort.desc, now), [leases.data, text, holder, sort, now]);

  const copy = (s: string) => void navigator.clipboard?.writeText(s).then(() => toast(t("copied")));
  const openAgent = (id: string) => dispatch({ type: "openWork", tab: { kind: "agent", agentId: id, ref: "" } });

  const cells = (l: Lease) => {
    const state = leaseState(l, now);
    const left = countdown(l, now);
    const soon = state === "active" && new Date(l.expires_at).getTime() - now.getTime() < Soon;
    const agent = actors.data?.get(l.holder_id);
    const releaser = l.released_by ? actors.data?.get(l.released_by) : undefined;
    return {
      resource: (
        <span className="flex items-center gap-1">
          <span className="truncate font-mono">{l.resource}</span>
          <IconButton size="sm" label={t("copy")} icon={<Copy className="size-3.5" />} onClick={() => copy(l.resource)} />
        </span>
      ),
      holder: (
        <button type="button" onClick={() => openAgent(l.holder_id)} className="flex items-center gap-1.5 hover:underline">
          <Avatar kind="agent" name={agent?.display_name || l.holder_handle} emoji={agent?.avatar_emoji} size="sm" />@{l.holder_handle}
        </button>
      ),
      note: <span className="line-clamp-2 text-fg-muted">{l.note}</span>,
      acquired: <time title={fullTime(l.acquired_at, locale)}>{relativeTime(l.acquired_at, now, locale)}</time>,
      expires: (
        <time title={fullTime(l.expires_at, locale)} className={cn("tabular-nums", soon && "font-semibold text-warning", state === "expired" && "font-semibold text-danger")}>
          {state === "released" ? "–" : (left ?? t("expiredAgo"))}
        </time>
      ),
      left: (
        <span aria-hidden className="block h-1.5 w-20 overflow-hidden rounded-full bg-surface-sunken">
          <span className={cn("block h-full rounded-full", soon ? "bg-warning" : "bg-success")} style={{ width: `${remaining(l, now) * 100}%` }} />
        </span>
      ),
      state: (
        <span className="flex flex-col items-start gap-0.5">
          <Badge tone={tone[state]}>{t(`state.${state}`)}</Badge>
          {state === "released" && l.released_at ? (
            <span className="text-meta text-fg-subtle">{t("releasedBy", { handle: releaser?.handle ?? "?", when: relativeTime(l.released_at, now, locale) })}</span>
          ) : null}
        </span>
      ),
      action:
        human && state !== "released" ? (
          <Button size="sm" icon={<Unlock className="size-3.5" />} onClick={() => setConfirm(l)}>
            {t("release")}
          </Button>
        ) : null,
    };
  };

  const header = (c: (typeof columns)[number]) => {
    if (c === "left" || c === "action") return <span className={c === "action" ? "sr-only" : undefined}>{t(`cols.${c}`)}</span>;
    const on = sort.key === c;
    return (
      <button type="button" aria-label={t("sortBy", { col: t(`cols.${c}`) })} onClick={() => setSort({ key: c, desc: on ? !sort.desc : false })} className="flex items-center gap-1 hover:text-fg">
        {t(`cols.${c}`)}
        {on ? sort.desc ? <ArrowDown aria-hidden className="size-3" /> : <ArrowUp aria-hidden className="size-3" /> : null}
      </button>
    );
  };

  return (
    <div className="flex h-full flex-col">
      <PageHeader icon={<KeyRound className="size-5" />} title={t("title")} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <div className="flex flex-wrap items-center gap-2">
          <div role="radiogroup" aria-label={t("filters")} className="flex rounded-md border border-border p-0.5">
            {[false, true].map((h) => (
              <button
                key={String(h)}
                type="button"
                role="radio"
                aria-checked={history === h}
                onClick={() => setHistory(h)}
                className={cn("rounded-sm px-2.5 py-1 text-ui", history === h ? "bg-accent-soft font-semibold text-fg" : "text-fg-muted")}
              >
                {t(h ? "history" : "active")}
              </button>
            ))}
          </div>
          <Select aria-label={t("holder")} value={holder} onChange={(e) => setHolder(e.target.value)} className="w-44">
            <option value="">{t("everyone")}</option>
            {holders.map(([id, handle]) => (
              <option key={id} value={id}>
                @{handle}
              </option>
            ))}
          </Select>
          <label className="relative min-w-48 flex-1">
            <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
            <Input type="search" aria-label={t("search")} placeholder={t("search")} value={text} onChange={(e) => setText(e.target.value)} className="pl-8" />
          </label>
        </div>
        {leases.isPending ? <Skeleton className="h-6 w-2/3" /> : null}
        {leases.data && rows.length === 0 ? <EmptyState icon={<KeyRound className="size-8" />} title={t(history ? "emptyHistory" : "empty")} /> : null}
        {rows.length && narrow ? (
          <ul className="flex flex-col gap-2">
            {rows.map((l) => {
              const c = cells(l);
              return (
                <li key={l.id} className={cn("flex flex-col gap-1.5 rounded-lg border border-border p-3 text-ui motion-safe:transition-colors", flash.has(l.id) && "bg-accent-soft")}>
                  <div className="flex items-start justify-between gap-2">
                    {c.resource}
                    {c.state}
                  </div>
                  {c.holder}
                  {c.note}
                  <div className="flex items-center gap-3 text-meta text-fg-muted">
                    {c.acquired} · {c.expires} {c.left}
                  </div>
                  {c.action}
                </li>
              );
            })}
          </ul>
        ) : rows.length ? (
          <div className="overflow-x-auto rounded-lg border border-border">
            <table className="w-full min-w-[60rem] text-left text-ui">
              <thead className="sticky top-0 z-10 bg-surface-sunken text-meta text-fg-muted">
                <tr>
                  {columns.map((c) => (
                    <th key={c} scope="col" aria-sort={sort.key === c ? (sort.desc ? "descending" : "ascending") : undefined} className="px-3 py-2 font-semibold">
                      {header(c)}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {rows.map((l) => {
                  const c = cells(l);
                  return (
                    <tr key={l.id} className={cn("align-middle motion-safe:transition-colors motion-safe:duration-700", flash.has(l.id) && "bg-accent-soft")}>
                      {columns.map((k) => (
                        <td key={k} className={cn("px-3 py-2", k === "resource" && "max-w-64", k === "note" && "max-w-56")}>
                          {c[k]}
                        </td>
                      ))}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : null}
      </div>
      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t("releaseTitle")}
        description={confirm ? t("releaseBody", { resource: confirm.resource, holder: confirm.holder_handle }) : undefined}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>{t("cancel")}</Button>
            <Button variant="danger" loading={release.isPending} onClick={() => confirm && release.mutate(confirm.id, { onSuccess: () => setConfirm(null) })}>
              {t("release")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </div>
  );
}
