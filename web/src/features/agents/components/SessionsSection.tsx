import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { cn, sessionContext } from "@/shared/lib";
import { contextPercent, nearRotation, placeHref, shortTokens, type OverviewThread } from "../model";
import { Section } from "./Section";

const SHOWN = 5;

/**
 * 2. Munkamenetek: each thread session with its context bar (amber from
 * 80 %). The server lists them by their latest run, newest first.
 */
export function SessionsSection({ threads, channelName }: { threads: OverviewThread[]; channelName: (id: string, fallback?: string) => string }) {
  const { t } = useTranslation("agents");
  const [all, setAll] = useState(false);
  const shown = all ? threads : threads.slice(0, SHOWN);
  return (
    <Section id="sessions" title={t("overview.sessions.title")}>
      {!threads.length ? <p className="text-fg-muted">{t("overview.sessions.empty")}</p> : null}
      <ul className="flex flex-col gap-2">
        {shown.map((th) => {
          const c = sessionContext(th);
          const pct = contextPercent(th);
          return (
            <li key={th.session_id} className="rounded-md border border-border p-2">
              <div className="flex items-baseline justify-between gap-2">
                <Link to={placeHref(th.channel_id, th.thread_root_id)} className="min-w-0 truncate text-accent-text hover:underline">
                  #{channelName(th.channel_id, th.channel_name)}
                  {th.thread_root_id ? <span className="text-fg-muted"> · {t("overview.now.thread")}</span> : null}
                </Link>
                <span className={cn("tabular shrink-0 text-meta", c.near ? "text-warning" : "text-fg-muted")}>
                  {t("overview.sessions.context")} {c.text}
                </span>
              </div>
              <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-surface-sunken" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(pct)} aria-label={t("overview.sessions.context")}>
                <div className={cn("h-full rounded-full", c.near ? "bg-warning" : "bg-fg-muted")} style={{ width: `${pct}%` }} />
              </div>
              <div className="mt-1 flex justify-between gap-2 text-meta">
                <span className="text-fg-subtle">{t("overview.sessions.lastRun", { tokens: shortTokens(th.last_run_tokens) })}</span>
                {nearRotation(th) ? <span className="font-semibold text-warning">{t("overview.sessions.near")}</span> : null}
              </div>
            </li>
          );
        })}
      </ul>
      {threads.length > SHOWN ? (
        <button type="button" onClick={() => setAll(!all)} className="mt-2 text-meta text-accent-text hover:underline">
          {all ? t("overview.sessions.less") : t("overview.sessions.all", { count: threads.length })}
        </button>
      ) : null}
    </Section>
  );
}
