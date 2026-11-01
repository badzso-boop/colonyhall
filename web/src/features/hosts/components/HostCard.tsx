import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { AlertTriangle, Ban, FolderX, History } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime, hueVar, relativeTime } from "@/shared/lib";
import { Badge } from "@/shared/ui";
import type { Host } from "../api";
import { hostWarnings, isOnline, type HostWarning } from "../model";
import { LimitRing } from "./bits";

/** Online, or when the machine was last seen ("utoljára 12 perce"). */
export function OnlineBadge({ host, now }: { host: Host; now: Date }) {
  const { t, i18n } = useTranslation("hosts");
  const locale = localeOf(i18n.language);
  if (isOnline(host, now)) return <Badge tone="success">{t("online")}</Badge>;
  return (
    <Badge tone="neutral">
      <span title={host.last_seen_at ? fullTime(host.last_seen_at, locale) : undefined}>
        {host.last_seen_at ? t("lastSeen", { when: relativeTime(host.last_seen_at, now, locale) }) : t("neverSeen")}
      </span>
    </Badge>
  );
}

const warningIcon = { revoked: Ban, rejected: AlertTriangle, exchange: FolderX, oldGuard: History } as const;

export function WarningBadge({ w }: { w: HostWarning }) {
  const { t } = useTranslation("hosts");
  const Icon = warningIcon[w.kind];
  const text = w.kind === "rejected" ? t("warnings.rejected", { count: w.count }) : w.kind === "oldGuard" ? t("warnings.oldGuard", { version: w.version }) : t(`warnings.${w.kind}`);
  return (
    <Badge tone={w.kind === "revoked" ? "danger" : "warning"}>
      <Icon aria-hidden className="size-3" />
      {text}
    </Badge>
  );
}

/** One machine in the list: colour stripe, name, state, report summary, warnings. */
export function HostCard({ host, now, selected }: { host: Host; now: Date; selected: boolean }) {
  const { t } = useTranslation("hosts");
  const warnings = hostWarnings(host);
  const r = host.report;
  return (
    <Link
      to={`/hosts/${host.id}`}
      aria-current={selected ? "page" : undefined}
      data-testid={`host-${host.id}`}
      style={{ borderLeftColor: hueVar(host.color) }}
      className={cn(
        "block rounded-md border border-l-[3px] border-border bg-surface-raised px-3 py-2 hover:bg-surface-sunken",
        selected && "bg-accent-soft hover:bg-accent-soft",
        host.state === "revoked" && "opacity-75",
      )}
    >
      <div className="flex items-center gap-2">
        <span className="min-w-0 flex-1 truncate font-semibold">{host.name}</span>
        <LimitRing limits={r.claude_limits} />
        <OnlineBadge host={host} now={now} />
      </div>
      <p className="mt-0.5 truncate text-meta text-fg-muted">
        {[r.os || t("unknownOs"), r.guard_version ? t("guardVersion", { version: r.guard_version }) : null, t("agentCount", { count: host.agents.length })]
          .filter(Boolean)
          .join(" · ")}
      </p>
      {warnings.length ? (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {warnings.map((w) => (
            <WarningBadge key={w.kind} w={w} />
          ))}
        </div>
      ) : null}
    </Link>
  );
}
