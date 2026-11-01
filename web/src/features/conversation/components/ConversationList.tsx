import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { ChevronDown, ChevronRight, Eye, Hash, Lock, Plus } from "lucide-react";
import { cn } from "@/shared/lib";
import { useLocalStorage } from "@/shared/hooks";
import { Avatar, Badge, Tooltip } from "@/shared/ui";
import { useActivity } from "@/features/agents";
import { useActors, useSession } from "@/features/identity";
import { useChannels, useSections } from "../api";
import { NewConversationDialog } from "./NewConversationDialog";
import { buildSidebar, dmPeers, unreadTotals, type Actor, type SidebarGroup, type SidebarRow } from "../model";

function RowIcon({ row, actors, meId }: { row: SidebarRow; actors: Map<string, Actor>; meId: string }) {
  const { t } = useTranslation("conversation");
  const working = useActivity((s) => s.working);
  if (row.kind === "channel") {
    return row.channel.kind === "private" ? (
      <Lock aria-label={t("row.private")} className="size-4 shrink-0 text-fg-subtle" />
    ) : (
      <Hash aria-hidden className="size-4 shrink-0 text-fg-subtle" />
    );
  }
  const peers = dmPeers(row.channel, actors, meId);
  if (row.kind === "agent-agent") {
    return (
      <span className="flex shrink-0">
        {peers.slice(0, 2).map((a, i) => (
          <Avatar key={a.id} kind="agent" name={a.handle} emoji={a.avatar_emoji} size="sm" className={i ? "-ml-1.5" : ""} />
        ))}
      </span>
    );
  }
  const peer = peers.find((a) => a.kind === "agent") ?? peers[0];
  if (!peer) return <Avatar kind="human" name={row.title} size="sm" />;
  return (
    <Avatar
      kind={peer.kind}
      name={peer.kind === "agent" ? peer.handle : peer.display_name || peer.handle}
      emoji={peer.avatar_emoji}
      size="sm"
      status={peer.kind === "agent" ? (working[peer.id] ? "working" : peer.state === "paused" ? "paused" : "idle") : undefined}
    />
  );
}

function Row({ row, actors, meId }: { row: SidebarRow; actors: Map<string, Actor>; meId: string }) {
  const { t } = useTranslation("conversation");
  const unread = row.channel.unread > 0;
  return (
    <NavLink
      to={`/c/${row.channel.id}`}
      className={({ isActive }) =>
        cn(
          "relative flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
          unread && "font-semibold text-fg",
          isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
        )
      }
    >
      <RowIcon row={row} actors={actors} meId={meId} />
      <span className={cn("min-w-0 flex-1 truncate", row.kind !== "channel" && row.kind !== "human-dm" && "font-mono text-meta", row.kind === "agent-agent" && "text-fg-muted")}>
        {row.title}
      </span>
      {row.observed ? (
        <Tooltip content={t("row.observed")}>
          <Eye aria-label={t("row.observed")} className="size-3.5 shrink-0 text-fg-subtle" />
        </Tooltip>
      ) : null}
      {row.channel.mentions > 0 ? (
        <Badge tone="accent" className="h-4.5 min-w-4.5">
          <span className="sr-only">{t("row.mentions", { count: row.channel.mentions })}</span>
          <span aria-hidden>{row.channel.mentions}</span>
        </Badge>
      ) : null}
    </NavLink>
  );
}

function Group({ group, actors, meId }: { group: SidebarGroup; actors: Map<string, Actor>; meId: string }) {
  const { t } = useTranslation("conversation");
  const title = group.title ?? t(`groups.${group.id}`);
  const [collapsed, setCollapsed] = useLocalStorage(`colony.sidebar.collapsed.${group.id}`, group.collapsed);
  const [adding, setAdding] = useState(false);
  if (!group.custom && group.rows.length === 0 && group.id === "agentAgent") return null;
  const totals = unreadTotals(group.rows);
  const addLabel = group.id === "channels" ? t("actions.newChannel") : group.id === "dms" ? t("actions.newDm") : group.id === "agentDms" ? t("actions.newAgentDm") : null;
  return (
    <section className="mt-3">
      <div className="flex h-7 items-center gap-1 pr-1 pl-1">
        <button
          type="button"
          aria-expanded={!collapsed}
          aria-label={t("actions.toggle", { name: title })}
          onClick={() => setCollapsed(!collapsed)}
          className="flex min-w-0 flex-1 items-center gap-1 rounded-sm text-meta font-semibold tracking-wide text-fg-subtle uppercase hover:text-fg-muted"
        >
          {collapsed ? <ChevronRight aria-hidden className="size-3.5" /> : <ChevronDown aria-hidden className="size-3.5" />}
          <span className="truncate">{title}</span>
        </button>
        {collapsed && totals.mentions > 0 ? <Badge tone="accent">{totals.mentions}</Badge> : null}
        {collapsed && totals.mentions === 0 && totals.unread > 0 ? <Badge>{totals.unread}</Badge> : null}
        {addLabel ? (
          <button
            type="button"
            aria-label={addLabel}
            title={addLabel}
            onClick={() => setAdding(true)}
            className="grid size-6 place-items-center rounded-sm text-fg-subtle hover:bg-surface-sunken hover:text-fg"
          >
            <Plus aria-hidden className="size-3.5" />
          </button>
        ) : null}
        {adding ? <NewConversationDialog kind={group.id === "channels" ? "channel" : group.id === "dms" ? "dm" : "agentDm"} meId={meId} onClose={() => setAdding(false)} /> : null}
      </div>
      {collapsed ? null : (
        <div className="flex flex-col gap-px pl-2">
          {group.rows.map((r) => (
            <Row key={r.channel.id} row={r} actors={actors} meId={meId} />
          ))}
        </div>
      )}
    </section>
  );
}

/** The conversation groups of the sidebar (docs/handoff/31, part 4). */
export function ConversationList() {
  const { t } = useTranslation("conversation");
  const channels = useChannels();
  const sections = useSections();
  const actors = useActors();
  const session = useSession();
  const meId = session.data?.actor.id ?? "";
  const groups = useMemo(
    () => (channels.data && actors.data ? buildSidebar(channels.data, sections.data ?? [], actors.data, meId) : []),
    [channels.data, sections.data, actors.data, meId],
  );
  if (!actors.data) return null;
  const fixed = groups.filter((g) => !g.custom);
  const ai = fixed.filter((g) => g.id === "agentDms" || g.id === "agentAgent");
  return (
    <nav aria-label={t("groups.channels")}>
      {groups.filter((g) => g.custom).map((g) => (
        <Group key={g.id} group={g} actors={actors.data} meId={meId} />
      ))}
      {fixed.filter((g) => g.id === "channels" || g.id === "dms").map((g) => (
        <Group key={g.id} group={g} actors={actors.data} meId={meId} />
      ))}
      <p className="mt-4 px-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("groups.ai")}</p>
      {ai.map((g) => (
        <Group key={g.id} group={g} actors={actors.data} meId={meId} />
      ))}
    </nav>
  );
}

/** Mentions across all conversations (the tab title and favicon dot). */
export function useMentionTotal(): number {
  const channels = useChannels();
  return (channels.data ?? []).reduce((n, c) => n + c.mentions, 0);
}

/** The conversations in sidebar order (keyboard navigation, the command palette). */
export function useConversationOrder(): SidebarRow[] {
  const channels = useChannels();
  const sections = useSections();
  const actors = useActors();
  const session = useSession();
  const meId = session.data?.actor.id ?? "";
  return useMemo(
    () => (channels.data && actors.data ? buildSidebar(channels.data, sections.data ?? [], actors.data, meId).flatMap((g) => g.rows) : []),
    [channels.data, sections.data, actors.data, meId],
  );
}
