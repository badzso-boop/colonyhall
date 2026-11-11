import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { CircleAlert, CircleCheck, Copy, Gauge, TriangleAlert } from "lucide-react";
import { useActors } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime } from "@/shared/lib";
import { Button, Notice, Skeleton, toast } from "@/shared/ui";
import { useSystemStatus } from "../api";
import { AdminPage } from "../components/AdminPage";
import { Sparkline } from "../components/charts";
import { EmergencyReleaseDialog } from "../components/EmergencyReleaseDialog";
import { formatBytes, formatUptime, LAG_EVENTS, LAG_SECONDS, sizeGrowth, statusFindings, statusText, type SystemStatus, type Tone } from "../model";

const toneIcon = { ok: CircleCheck, warning: TriangleAlert, error: CircleAlert };
const toneClass = { ok: "text-success", warning: "text-warning", error: "text-danger" };

function StatusCard({ title, tone, children }: { title: string; tone: Tone; children: ReactNode }) {
  const { t } = useTranslation("admin");
  const Icon = toneIcon[tone];
  return (
    <section className={cn("flex min-w-0 flex-col gap-2 rounded-lg border bg-surface p-3", tone === "ok" ? "border-border" : tone === "warning" ? "border-warning" : "border-danger")}>
      <h2 className="flex items-center gap-2 font-semibold">
        <Icon aria-hidden className={cn("size-4 shrink-0", toneClass[tone])} />
        <span className="min-w-0 flex-1 truncate">{title}</span>
        <span className={cn("text-meta font-medium", toneClass[tone])}>{t(`status.tone.${tone}`)}</span>
      </h2>
      <div className="flex flex-col gap-1 text-ui text-fg-muted">{children}</div>
    </section>
  );
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <p className="flex flex-wrap justify-between gap-x-3">
      <span>{label}</span>
      <span className="tabular min-w-0 font-medium break-all text-fg">{value}</span>
    </p>
  );
}

/** Rendszerállapot (/admin/status): the first place to look; re-read every 10 s while the tab is visible. */
export default function StatusPage() {
  const { t } = useTranslation("admin");
  return (
    <AdminPage icon={<Gauge className="size-5" />} title={t("status.title")}>
      <StatusBody />
    </AdminPage>
  );
}

function StatusBody() {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const status = useSystemStatus();
  const actors = useActors();
  const [release, setRelease] = useState(false);
  if (status.isPending) return <Skeleton className="m-4 h-64" />;
  if (status.isError) return <div className="p-4"><Notice tone="danger">{t("status.unreachable", { message: status.error.message })}</Notice></div>;
  const st = status.data;
  const s = st.system;
  if (!s) return null;
  const tones = Object.fromEntries(statusFindings(st).map((f) => [f.id, f.tone])) as Record<string, Tone>;
  const tone = (id: string): Tone => tones[id] ?? "ok";
  const yes = (on: boolean) => t(on ? "settings.yes" : "settings.no");
  const when = (at: string | undefined) => (at ? fullTime(at, i18n.language) : "–");
  const stoppedBy = st.emergency_stop.by ? actors.data?.get(st.emergency_stop.by) : undefined;
  const copy = () => {
    navigator.clipboard.writeText(statusText(st, new Date())).then(
      () => toast(t("copied")),
      () => toast(t("copyFailed"), { tone: "danger" }),
    );
  };
  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-surface-sunken p-3 sm:p-4">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <p className="mr-auto text-meta text-fg-muted">{t("status.refresh")}</p>
        <Button size="sm" icon={<Copy className="size-4" />} onClick={copy}>
          {t("status.copy")}
        </Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <StatusCard title={t("status.server")} tone={tone("server")}>
          <Row label={t("status.version")} value={s.version} />
          <Row label={t("status.uptime")} value={formatUptime(s.uptime_seconds)} />
        </StatusCard>
        <StatusCard title={t("status.database")} tone={tone("database")}>
          <Row label={t("status.reachable")} value={yes(s.database.ok)} />
          <Row label={t("status.latency")} value={`${s.database.latency_ms.toLocaleString(locale, { maximumFractionDigits: 1 })} ms`} />
        </StatusCard>
        <DatabaseSizeCard database={s.database} />
        <StatusCard title={t("status.emergency")} tone={tone("emergency")}>
          {st.emergency_stop.active ? (
            <>
              <Row label={t("status.stoppedBy")} value={stoppedBy ? `@${stoppedBy.handle}` : "–"} />
              <Row label={t("status.stoppedAt")} value={when(st.emergency_stop.since)} />
              <div>
                <Button size="sm" variant="primary" onClick={() => setRelease(true)}>
                  {t("emergency.release")}
                </Button>
              </div>
            </>
          ) : (
            <p>{t("status.notStopped")}</p>
          )}
        </StatusCard>
        <StatusCard title={t("status.events")} tone={tone("events")}>
          <Row label={t("status.head")} value={s.event_head.toLocaleString(locale)} />
          {s.consumers.map((c) => {
            const late = c.lag > LAG_EVENTS || c.lag_seconds > LAG_SECONDS;
            return <Row key={c.name} label={c.name} value={<span className={cn(late && "text-warning")}>{t("status.lag", { events: c.lag.toLocaleString(locale), seconds: c.lag_seconds })}</span>} />;
          })}
        </StatusCard>
        <StatusCard title={t("status.guards")} tone={tone("guards")}>
          {s.guards.length ? (
            s.guards.map((g) => (
              <Row
                key={g.id}
                label={g.name}
                value={
                  <span className={cn(!g.online && "text-warning")}>
                    {t(g.online ? "status.online" : "status.offline")} · {g.guard_version ?? "?"} · {when(g.last_seen_at)}
                  </span>
                }
              />
            ))
          ) : (
            <p>{t("status.noGuards")}</p>
          )}
        </StatusCard>
        <StatusCard title={t("status.connections")} tone={tone("connections")}>
          <Row label={t("status.humans")} value={s.connections.human} />
          <Row label={t("status.agents")} value={s.connections.agent} />
          <Row label={t("status.hosts")} value={s.connections.host} />
        </StatusCard>
        <StatusCard title={t("status.scheduler")} tone={tone("scheduler")}>
          <Row label={t("status.nextRun")} value={when(s.scheduler.next_run_at)} />
          <Row label={t("status.failed24h")} value={s.scheduler.failed_24h} />
        </StatusCard>
        <StatusCard title={t("status.compaction")} tone={tone("compaction")}>
          <Row label={t("status.running")} value={s.compaction.running} />
          <Row label={t("status.failed24h")} value={s.compaction.failed_24h} />
        </StatusCard>
        <StatusCard title={t("status.integrations")} tone={tone("integrations")}>
          <Row label="Gitea" value={yes(s.gitea_configured)} />
          <Row label={t("settings.gitea.webhook")} value={yes(s.gitea_webhook_configured)} />
        </StatusCard>
        <section className="flex min-w-0 flex-col gap-1 rounded-lg border border-border bg-surface p-3">
          <h2 className="font-semibold">{t("status.backups")}</h2>
          <p className="text-ui text-fg-muted">{t("status.backupsBody")}</p>
        </section>
      </div>
      <EmergencyReleaseDialog open={release} onOpenChange={setRelease} />
    </div>
  );
}

/**
 * Adatbázis mérete (round 50): the total, the ten largest tables and the
 * growth over the last 30 days. The memory store has no database.
 */
export function DatabaseSizeCard({ database }: { database: SystemStatus["database"] }) {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const bytes = (n: number) => formatBytes(n, locale);
  if (database.size_bytes === undefined) {
    return (
      <StatusCard title={t("status.size.title")} tone="ok">
        <p>{t("status.size.noDatabase")}</p>
      </StatusCard>
    );
  }
  const growth = sizeGrowth(database.growth ?? []);
  const tables = database.tables ?? [];
  return (
    <StatusCard title={t("status.size.title")} tone="ok">
      <Row label={t("status.size.total")} value={bytes(database.size_bytes)} />
      <div className="flex flex-wrap items-center justify-between gap-x-3">
        <span>{t("status.size.growth")}</span>
        {growth ? (
          <span className="flex items-center gap-2">
            <span className="tabular font-medium text-fg">{t("status.size.growthText", { delta: `${growth.delta >= 0 ? "+" : ""}${bytes(growth.delta)}`, count: growth.days })}</span>
            <Sparkline values={growth.recent.map((x) => x.size_bytes)} label={t("status.size.growth")} format={bytes} fromMin />
          </span>
        ) : (
          <span className="text-fg-subtle">{t("status.size.noGrowth")}</span>
        )}
      </div>
      {tables.length ? (
        <table className="mt-1 w-full table-fixed text-meta">
          <caption className="sr-only">{t("status.size.tables")}</caption>
          <thead>
            <tr className="text-left text-fg-subtle">
              <th scope="col" className="w-1/2 font-medium">{t("status.size.table")}</th>
              <th scope="col" className="text-right font-medium">{t("status.size.tableSize")}</th>
              <th scope="col" className="text-right font-medium">{t("status.size.rows")}</th>
            </tr>
          </thead>
          <tbody>
            {tables.map((x) => (
              <tr key={x.name}>
                <td className="truncate font-mono text-fg">{x.name}</td>
                <td className="tabular text-right text-fg">{bytes(x.total_bytes)}</td>
                <td className="tabular text-right">~{x.rows_estimate.toLocaleString(locale)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </StatusCard>
  );
}
