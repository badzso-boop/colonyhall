import { useTranslation } from "react-i18next";
import { cn, sessionContext } from "@/shared/lib";
import { Avatar, Badge, EmptyState, Skeleton } from "@/shared/ui";
import { useActivity } from "../activity";
import { useAgentOverview } from "../api";

/**
 * The agent overview work pane, first version (round 33 completes it):
 * header with state, machine, client and access, and the sessions with
 * their context bars.
 */
export function AgentOverviewPane({ agentId }: { agentId: string }) {
  const { t } = useTranslation("agents");
  const overview = useAgentOverview(agentId);
  const working = useActivity((s) => Boolean(s.working[agentId]));
  if (overview.isError) return <EmptyState title={t("overview.notFound")} />;
  if (!overview.data) return <Skeleton className="m-4 h-6 w-1/2" />;
  const { actor, assignment, threads, usage } = overview.data;
  const state = working ? "working" : overview.data.state;
  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <header className="flex items-start gap-3 border-b border-border p-4">
        <Avatar kind={actor.kind} name={actor.handle} emoji={actor.avatar_emoji} size="lg" status={working ? "working" : state === "paused" ? "paused" : "idle"} />
        <div className="min-w-0 flex-1">
          <p className="text-title font-semibold">{actor.display_name}</p>
          <p className="font-mono text-meta text-fg-muted">@{actor.handle}</p>
          {actor.description ? <p className="mt-1 text-ui text-fg-muted">{actor.description}</p> : null}
          <div className="mt-2 flex flex-wrap gap-1.5">
            <Badge tone={working ? "accent" : "neutral"}>{t(`overview.states.${state}`, { defaultValue: state })}</Badge>
            {assignment ? <Badge>{assignment.host_name ?? assignment.host_id}</Badge> : null}
            {assignment ? <Badge>{`${assignment.client} · ${assignment.model}`}</Badge> : null}
            {assignment ? <Badge tone={assignment.sandbox ? "info" : "warning"}>{assignment.sandbox ? t("overview.sandbox") : t("overview.fullAccess")}</Badge> : null}
          </div>
        </div>
      </header>
      {usage ? (
        <p className="px-4 pt-3 text-ui text-fg-muted">
          {t("overview.today", { tokens: usage.today_tokens.toLocaleString() })}
          {usage.daily_token_limit > 0 ? ` · ${t("overview.limit", { limit: usage.daily_token_limit.toLocaleString() })}` : ""}
        </p>
      ) : null}
      <section className="p-4">
        <h3 className="mb-2 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("overview.sessions")}</h3>
        {!threads?.length ? <p className="text-ui text-fg-muted">{t("overview.noSessions")}</p> : null}
        <ul className="flex flex-col gap-2">
          {(threads ?? []).map((th) => {
            const c = sessionContext(th);
            const pct = th.max_session_tokens > 0 && th.context_tokens ? Math.min(100, (th.context_tokens / th.max_session_tokens) * 100) : 0;
            return (
              <li key={th.session_id} className="rounded-md border border-border p-2">
                <div className="flex items-baseline justify-between gap-2 text-ui">
                  <span className="truncate">#{th.channel_name ?? th.channel_id}</span>
                  <span className={cn("tabular text-meta", c.near ? "text-warning" : "text-fg-muted")} title={c.runTokens}>
                    {t("overview.context")} {c.text}
                  </span>
                </div>
                <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-surface-sunken">
                  <div className={cn("h-full rounded-full", c.near ? "bg-warning" : "bg-fg-muted")} style={{ width: `${pct}%` }} />
                </div>
                {c.near ? <p className="mt-1 text-meta text-warning">{t("overview.nearLimit")}</p> : null}
              </li>
            );
          })}
        </ul>
      </section>
    </div>
  );
}
