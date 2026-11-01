import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { MessageSquare, SquareTerminal } from "lucide-react";
import { useNow } from "@/shared/hooks";
import { hueVar } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Avatar, Button, EmptyState, Skeleton, toast } from "@/shared/ui";
import { useSession } from "@/features/identity";
import { useActivity } from "../activity";
import { useAgentOverview, useHosts, useOpenDm } from "../api";
import { displayState, type AgentOverview } from "../model";
import { useOpenSettings } from "../settingsLink";
import { CollaborationSection, CommonChannels } from "./CollaborationSection";
import { ConfigSection } from "./ConfigSection";
import { InstructionsSection, NotesSection } from "./NotesSection";
import { NowSection } from "./NowSection";
import { OverviewHeader } from "./OverviewHeader";
import { RunsSection } from "./RunsSection";
import { Section } from "./Section";
import { SessionsSection } from "./SessionsSection";
import { UsageSection } from "./UsageSection";

/** A human's overview: the profile and the channels in common (docs/handoff/33, part 3). */
function HumanOverview({ overview }: { overview: AgentOverview }) {
  const { t } = useTranslation("agents");
  const { actor } = overview;
  const navigate = useNavigate();
  const dm = useOpenDm();
  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <header className="flex items-start gap-3 border-b border-border p-4">
        <Avatar kind="human" name={actor.handle} emoji={actor.avatar_emoji} size="lg" className="size-14 text-page" />
        <div className="min-w-0 flex-1">
          <p className="text-title font-semibold">{actor.display_name}</p>
          <p className="font-mono text-meta text-fg-muted">@{actor.handle}</p>
          {actor.status_text ? <p className="mt-1 text-ui">{actor.status_text}</p> : null}
          {actor.description ? <p className="mt-1 text-ui text-fg-muted">{actor.description}</p> : null}
          <p className="mt-1 text-meta text-fg-subtle">
            {t("overview.human.role", { role: actor.role })}
            {actor.timezone ? ` · ${t("overview.human.timezone", { tz: actor.timezone })}` : ""}
          </p>
          <Button
            size="sm"
            className="mt-3"
            icon={<MessageSquare className="size-3.5" />}
            loading={dm.isPending}
            onClick={() => dm.mutate(actor.id, { onSuccess: (ch) => void navigate(`/c/${ch.id}`), onError: () => toast(t("overview.actions.dmFailed")) })}
          >
            {t("overview.actions.dm")}
          </Button>
        </div>
      </header>
      <div className="p-3">
        <Section id="human-channels" title={t("overview.collab.channels")}>
          <CommonChannels overview={overview} />
        </Section>
      </div>
    </div>
  );
}

function AgentView({ overview, terminals }: { overview: AgentOverview; terminals?: ReactNode }) {
  const { t } = useTranslation("agents");
  const session = useSession();
  const me = session.data?.actor;
  const admin = me?.role === "owner" || me?.role === "admin";
  const human = me?.kind === "human";
  const { actor, assignment } = overview;
  // Live frames win; the overview's own flag covers a run that started before
  // this page connected (an idle frame refetches the overview, see events.ts).
  const live = useActivity((s) => Boolean(s.working[actor.id]?.length));
  const working = live || Boolean(overview.activity?.working);
  const hosts = useHosts(admin && Boolean(assignment));
  const host = hosts.data?.find((h) => h.id === assignment?.host_id);
  const now = useNow();
  const state = displayState({ archived: Boolean(actor.archived_at), state: overview.state, working, hostLastSeen: host ? (host.last_seen_at ?? "") : undefined }, now);
  // The host's colour; without the host list, the agent's effective colour is the host's when it has none of its own.
  const hostColor = hueVar(host?.color ?? (actor.color ? undefined : actor.effective_color));
  const openSettings = useOpenSettings(actor.handle);
  const [, dispatch] = usePanes();
  const channelNames = new Map<string, string>();
  for (const c of overview.common_channels ?? []) if (c.name) channelNames.set(c.id, c.name);
  for (const th of overview.threads ?? []) if (th.channel_name) channelNames.set(th.channel_id, th.channel_name);
  const channelName = (id: string, fallback?: string) => fallback || channelNames.get(id) || t("overview.now.channel");
  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <OverviewHeader overview={overview} state={state} hostColor={hostColor} admin={admin} />
      <div className="flex flex-col gap-3 p-3">
        <NowSection overview={overview} channelName={channelName} />
        <SessionsSection threads={overview.threads ?? []} channelName={channelName} />
        <UsageSection overview={overview} human={human} />
        <RunsSection overview={overview} human={human} channelName={channelName} />
        <Section
          id="terminals"
          title={t("overview.terminals.title")}
          aside={
            <button type="button" onClick={() => dispatch({ type: "openWork", tab: { kind: "terminal", agentId: actor.id, ref: "" } })} className="flex shrink-0 items-center gap-1 text-meta text-accent-text hover:underline">
              <SquareTerminal aria-hidden className="size-3.5" />
              {t("overview.terminals.open")}
            </button>
          }
        >
          <div className="-mx-3 -my-3">{terminals}</div>
        </Section>
        <ConfigSection overview={overview} onDetails={() => openSettings("konfiguracio")} />
        <CollaborationSection overview={overview} admin={admin} />
        <NotesSection overview={overview} />
        {human ? <InstructionsSection agentId={actor.id} openSettings={openSettings} /> : null}
      </div>
    </div>
  );
}

/**
 * The agent overview work pane (docs/handoff/33, part 3): header, then the
 * collapsible sections. Humans get the small version. The terminal list
 * belongs to the terminals feature, which itself uses this one, so the layout
 * passes it in (`terminals`) instead of this feature importing it.
 */
export function AgentOverviewPane({ agentId, terminals }: { agentId: string; terminals?: ReactNode }) {
  const { t } = useTranslation("agents");
  const overview = useAgentOverview(agentId);
  if (overview.isError) return <EmptyState title={t("overview.notFound")} />;
  if (!overview.data) return <Skeleton className="m-4 h-6 w-1/2" />;
  if (overview.data.actor.kind !== "agent") return <HumanOverview overview={overview.data} />;
  return <AgentView overview={overview.data} terminals={terminals} />;
}
