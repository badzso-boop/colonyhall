import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle } from "lucide-react";
import { useActors } from "@/features/identity";
import { Badge, Notice } from "@/shared/ui";
import type { Host } from "../api";
import { clock, stamp } from "./bits";
import { OnlineBadge } from "./HostCard";

/** A label and its value in the detail's definition lists. */
export function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 py-2 sm:grid-cols-[12rem_1fr]">
      <dt className="text-ui text-fg-muted">{label}</dt>
      <dd className="min-w-0 text-body">{children}</dd>
    </div>
  );
}

export function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono text-ui break-all">{children}</span>;
}

/** Áttekintés: what the guard reported about the machine. */
export function OverviewTab({ host, now }: { host: Host; now: Date }) {
  const { t, i18n } = useTranslation("hosts");
  const actors = useActors();
  const r = host.report;
  const handle = (id: string) => {
    const a = host.agents.find((x) => x.agent_id === id);
    return a ? `@${a.handle}` : actors.data?.get(id) ? `@${actors.data.get(id)!.handle}` : id;
  };
  const windows = (["five_hour", "seven_day"] as const).filter((k) => r.claude_limits?.[k]);

  return (
    <div className="flex flex-col gap-4">
      {r.rejected.length ? (
        <Notice tone="warning" icon={<AlertTriangle className="size-4" />}>
          <p className="font-semibold">{t("overview.rejected")}</p>
          <ul className="mt-1 flex flex-col gap-1">
            {r.rejected.map((x, i) => (
              <li key={`${x.agent_id}:${x.path}:${i}`}>
                {handle(x.agent_id)}: <Mono>{x.path}</Mono> – {x.reason}
              </li>
            ))}
          </ul>
        </Notice>
      ) : null}
      <dl className="divide-y divide-border">
        <Row label={t("overview.state")}>
          <span className="flex flex-wrap items-center gap-2">
            <Badge tone={host.state === "revoked" ? "danger" : "success"}>{t(`states.${host.state}`)}</Badge>
            <OnlineBadge host={host} now={now} />
          </span>
        </Row>
        <Row label={t("overview.os")}>{r.os || "–"}</Row>
        <Row label={t("overview.guard")}>{r.guard_version || "–"}</Row>
        <Row label={t("overview.clients")}>
          {r.clients.length ? (
            <ul>
              {r.clients.map((c) => (
                <li key={c.name}>
                  <Mono>{c.name}</Mono> {c.version ? <span className="text-fg-muted">{c.version}</span> : null}
                </li>
              ))}
            </ul>
          ) : (
            t("overview.none")
          )}
        </Row>
        <Row label={t("overview.sandbox")}>{r.sandbox || t("overview.noSandbox")}</Row>
        <Row label={t("overview.allowUnsandboxed")}>{t(r.allow_unsandboxed ? "yes" : "no")}</Row>
        <Row label={t("overview.exchange")}>
          {r.exchange_dir ? (
            <span className="flex flex-wrap items-center gap-2">
              <Mono>{r.exchange_dir}</Mono>
              <Badge tone={r.exchange_ok ? "success" : "warning"}>{t(r.exchange_ok ? "overview.exchangeOk" : "overview.exchangeBad")}</Badge>
            </span>
          ) : (
            t("overview.none")
          )}
        </Row>
        <Row label={t("overview.roots")}>
          {r.roots.length ? (
            <ul>
              {r.roots.map((p) => (
                <li key={p}>
                  <Mono>{p}</Mono>
                </li>
              ))}
            </ul>
          ) : (
            t("overview.none")
          )}
        </Row>
        <Row label={t("overview.claude")}>
          {windows.length ? (
            <ul>
              {windows.map((k) => {
                const w = r.claude_limits![k]!;
                return (
                  <li key={k} className="tabular">
                    {t(`overview.${k}`, { pct: Math.round(w.utilization * 100), reset: (k === "five_hour" ? clock : stamp)(w.resets_at, i18n.language) })}
                  </li>
                );
              })}
            </ul>
          ) : (
            t("overview.noLimits")
          )}
        </Row>
      </dl>
    </div>
  );
}
