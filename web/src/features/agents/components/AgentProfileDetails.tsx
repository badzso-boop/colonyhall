import { useTranslation } from "react-i18next";
import { LayoutDashboard } from "lucide-react";
import { useNow } from "@/shared/hooks";
import { usePanes } from "@/shared/panes";
import { Badge, Button, Skeleton } from "@/shared/ui";
import { useActivity } from "../activity";
import { isNotFound, useAgentOverview } from "../api";
import { displayState } from "../model";

const stateTone = { working: "accent", idle: "neutral", offline: "neutral", paused: "warning", stopped: "danger", archived: "neutral" } as const;

/**
 * An agent's part of the profile popover (round 34, part 7): its
 * state, machine and client, and "Áttekintés", which opens the overview pane.
 */
export function AgentProfileDetails({ agentId, onOpenOverview }: { agentId: string; onOpenOverview?: () => void }) {
  const { t } = useTranslation("agents");
  const overview = useAgentOverview(agentId);
  const live = useActivity((s) => Boolean(s.working[agentId]?.length));
  const now = useNow();
  const [, dispatch] = usePanes();
  const o = overview.data;
  const open = () => {
    dispatch({ type: "openWork", tab: { kind: "agent", agentId, ref: "" } });
    onOpenOverview?.();
  };
  if (isNotFound(overview.error)) {
    // Another person's agent (round 38): its name shows, its details do not.
    return <p className="text-meta text-fg-muted">{t("profile.notYours")}</p>;
  }
  return (
    <div className="flex flex-col gap-2">
      {o ? (
        <dl className="grid grid-cols-[auto_1fr] items-center gap-x-3 gap-y-1.5 text-ui">
          <dt className="text-meta text-fg-subtle">{t("profile.state")}</dt>
          <dd>
            {(() => {
              const state = displayState({ archived: Boolean(o.actor.archived_at), state: o.state, working: live || Boolean(o.activity?.working) }, now);
              return <Badge tone={stateTone[state]}>{t(`overview.states.${state}`)}</Badge>;
            })()}
          </dd>
          {o.assignment ? (
            <>
              <dt className="text-meta text-fg-subtle">{t("profile.machine")}</dt>
              <dd className="truncate">{o.assignment.host_name ?? o.assignment.host_id}</dd>
              <dt className="text-meta text-fg-subtle">{t("profile.client")}</dt>
              <dd className="truncate font-mono text-meta">{`${o.assignment.client} · ${o.assignment.model}`}</dd>
            </>
          ) : null}
        </dl>
      ) : overview.isPending ? (
        <Skeleton className="h-10 w-full" />
      ) : null}
      <Button size="sm" icon={<LayoutDashboard aria-hidden className="size-3.5" />} onClick={open} className="self-start">
        {t("profile.overview")}
      </Button>
    </div>
  );
}
