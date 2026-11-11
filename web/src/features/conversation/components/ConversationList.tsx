import { createContext, useContext, useMemo, useState, type DragEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { ArrowDown, ArrowUp, BellOff, ChevronDown, ChevronRight, Eye, Hash, Lock, LockKeyhole, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react";
import { cn } from "@/shared/lib";
import { useLocalStorage, useNow } from "@/shared/hooks";
import { Avatar, Badge, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, Tooltip, toast } from "@/shared/ui";
import { useActivity } from "@/features/agents";
import { useActors, useSession } from "@/features/identity";
import { useChannels, useMoveConversation, useReorderSections, useSections } from "../api";
import { isMuted } from "../channel";
import { NewConversationDialog } from "./NewConversationDialog";
import { ProfileAnchor } from "./ProfilePopover";
import { RowMenu } from "./RowMenu";
import { DeleteSectionDialog, SectionNameDialog } from "./SectionDialogs";
import { buildSidebar, dmPeers, unreadTotals, type Actor, type Section, type SidebarGroup, type SidebarRow } from "../model";
import { dropIndex, itemOrder, sectionOf, sectionOrder } from "../sections";

/* ---------- Drag and drop (round 34, part 2) ---------- */

type Drag = { type: "section"; id: string } | { type: "row"; id: string; from: string | null };

type Dnd = {
  drag: Drag | null;
  over: string | null;
  start: (d: Drag, e: DragEvent) => void;
  /** Props for a drop target: `key` marks the indicator, `accepts` filters, `drop` acts. */
  target: (key: string, accepts: (d: Drag) => boolean, drop: (d: Drag) => void) => {
    onDragOver: (e: DragEvent) => void;
    onDragLeave: (e: DragEvent) => void;
    onDrop: (e: DragEvent) => void;
    "data-drop-over"?: true;
  };
};

const DndContext = createContext<Dnd | null>(null);
const useDnd = () => useContext(DndContext)!;

function DndProvider({ children }: { children: ReactNode }) {
  const [drag, setDrag] = useState<Drag | null>(null);
  const [over, setOver] = useState<string | null>(null);
  const end = () => {
    setDrag(null);
    setOver(null);
  };
  const value: Dnd = {
    drag,
    over,
    start: (d, e) => {
      e.stopPropagation();
      e.dataTransfer.effectAllowed = "move";
      // Firefox only starts a drag that carries some data.
      e.dataTransfer.setData("text/plain", d.id);
      setDrag(d);
    },
    target: (key, accepts, drop) => ({
      onDragOver: (e) => {
        if (!drag || !accepts(drag)) return;
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = "move";
        if (over !== key) setOver(key);
      },
      onDragLeave: (e) => {
        if (over === key && !e.currentTarget.contains(e.relatedTarget as Node | null)) setOver(null);
      },
      onDrop: (e) => {
        if (!drag || !accepts(drag)) return;
        e.preventDefault();
        e.stopPropagation();
        drop(drag);
        end();
      },
      ...(over === key ? { "data-drop-over": true as const } : {}),
    }),
  };
  return (
    <DndContext.Provider value={value}>
      <div onDragEnd={end}>{children}</div>
    </DndContext.Provider>
  );
}

const dropClass = "data-drop-over:bg-accent-soft data-drop-over:ring-1 data-drop-over:ring-accent";

/* ---------- Rows ---------- */

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

type RowProps = { row: SidebarRow; actors: Map<string, Actor>; meId: string; sections: Section[]; sectionId: string | null; human: boolean };

function Row({ row, actors, meId, sections, sectionId, human }: RowProps) {
  const { t } = useTranslation("conversation");
  const dnd = useDnd();
  const move = useMoveConversation();
  const [profileOpen, setProfileOpen] = useState(false);
  const channelId = row.channel.id;
  const muted = isMuted(row.channel, useNow(60_000));
  // A muted row is never bold for unread messages; a mention still shows its badge.
  const unread = row.channel.unread > 0 && !muted;
  // A DM with one other person or agent: its row menu offers the profile.
  const peers = row.kind === "human-dm" || row.kind === "agent-dm" ? dmPeers(row.channel, actors, meId) : [];
  const peerId = peers.length === 1 ? peers[0]!.id : null;
  // Inside a section, a row is a drop target: the dragged conversation lands at its place.
  const drop = sectionId
    ? dnd.target(
        `row:${channelId}`,
        (d) => d.type === "row" && d.id !== channelId,
        (d) => {
          if (d.type !== "row") return;
          const index = dropIndex(itemOrder(sections, sectionId), d.id, channelId);
          move.mutate({ channelId: d.id, from: d.from, target: sectionId, index }, { onError: () => toast(t("sections.failed"), { tone: "danger" }) });
        },
      )
    : {};
  return (
    <ProfileAnchor actorId={peerId} open={profileOpen} onOpenChange={setProfileOpen}>
      <div
        className={cn("group/row relative rounded-md", dropClass, dnd.drag?.type === "row" && dnd.drag.id === channelId && "opacity-50")}
        draggable={human}
        onDragStart={human ? (e) => dnd.start({ type: "row", id: channelId, from: sectionId }, e) : undefined}
        {...drop}
      >
        <NavLink
          to={`/c/${channelId}`}
          draggable={false}
          className={({ isActive }) =>
            cn(
              "relative flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
              human && "pr-8",
              unread && "font-semibold text-fg",
              muted && "text-fg-subtle",
              isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
            )
          }
        >
          <RowIcon row={row} actors={actors} meId={meId} />
          <span className={cn("min-w-0 flex-1 truncate", row.kind !== "channel" && row.kind !== "human-dm" && "font-mono text-meta", row.kind === "agent-agent" && "text-fg-muted")}>
            {row.title}
          </span>
          {muted ? <BellOff aria-label={t("mute.muted")} className="size-3.5 shrink-0 text-fg-subtle" /> : null}
          {row.channel.agent_locked_at ? <LockKeyhole aria-label={t("channel.lock.locked")} className="size-3.5 shrink-0 text-warning" /> : null}
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
        {human ? <RowMenu row={row} sections={sections} sectionId={sectionId} onShowProfile={peerId ? () => setProfileOpen(true) : undefined} /> : null}
      </div>
    </ProfileAnchor>
  );
}

/* ---------- Groups ---------- */

type GroupProps = { group: SidebarGroup; actors: Map<string, Actor>; meId: string; sections: Section[]; human: boolean };

function SectionMenu({ section, sections }: { section: Section; sections: Section[] }) {
  const { t } = useTranslation("conversation");
  const [dialog, setDialog] = useState<"rename" | "delete" | null>(null);
  const reorder = useReorderSections();
  const order = sectionOrder(sections);
  const at = order.indexOf(section.id);
  const shift = (by: -1 | 1) => reorder.mutate({ sectionId: section.id, toIndex: at + by }, { onError: () => toast(t("sections.failed"), { tone: "danger" }) });
  return (
    <>
      <Menu>
        <MenuTrigger asChild>
          <button
            type="button"
            aria-label={t("sections.menu", { name: section.name })}
            title={t("sections.menu", { name: section.name })}
            className="grid size-6 place-items-center rounded-sm text-fg-subtle opacity-0 group-hover/group:opacity-100 group-focus-within/group:opacity-100 hover:bg-surface-sunken hover:text-fg data-[state=open]:opacity-100 pointer-coarse:opacity-100"
          >
            <MoreHorizontal aria-hidden className="size-3.5" />
          </button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem icon={<Pencil aria-hidden className="size-4" />} onSelect={() => setDialog("rename")}>
            {t("sections.rename")}
          </MenuItem>
          {at > 0 ? (
            <MenuItem icon={<ArrowUp aria-hidden className="size-4" />} onSelect={() => shift(-1)}>
              {t("sections.moveUp")}
            </MenuItem>
          ) : null}
          {at < order.length - 1 ? (
            <MenuItem icon={<ArrowDown aria-hidden className="size-4" />} onSelect={() => shift(1)}>
              {t("sections.moveDown")}
            </MenuItem>
          ) : null}
          <MenuSeparator />
          <MenuItem danger icon={<Trash2 aria-hidden className="size-4" />} onSelect={() => setDialog("delete")}>
            {t("sections.delete")}
          </MenuItem>
        </MenuContent>
      </Menu>
      {dialog === "rename" ? <SectionNameDialog section={section} onClose={() => setDialog(null)} /> : null}
      {dialog === "delete" ? <DeleteSectionDialog section={section} onClose={() => setDialog(null)} /> : null}
    </>
  );
}

function Group({ group, actors, meId, sections, human }: GroupProps) {
  const { t } = useTranslation("conversation");
  const dnd = useDnd();
  const move = useMoveConversation();
  const reorder = useReorderSections();
  const title = group.title ?? t(`groups.${group.id}`);
  const [collapsed, setCollapsed] = useLocalStorage(`colony.sidebar.collapsed.${group.id}`, group.collapsed);
  const [adding, setAdding] = useState(false);
  const section = group.custom ? sections.find((s) => s.id === group.id) : undefined;
  const failed = () => toast(t("sections.failed"), { tone: "danger" });
  // A section takes rows (appended) and other sections (put at its place);
  // a default group takes rows back out of their section.
  const drop = human
    ? dnd.target(
        `group:${group.id}`,
        (d) => (group.custom ? d.type === "row" || d.id !== group.id : d.type === "row" && d.from !== null),
        (d) => {
          if (d.type === "section") {
            reorder.mutate({ sectionId: d.id, toIndex: sectionOrder(sections).indexOf(group.id) }, { onError: failed });
          } else if (group.custom) {
            if (sectionOf(sections, d.id) !== group.id) move.mutate({ channelId: d.id, from: d.from, target: group.id }, { onError: failed });
          } else {
            move.mutate({ channelId: d.id, from: d.from, target: null }, { onError: failed });
          }
        },
      )
    : {};
  if (!group.custom && group.rows.length === 0 && group.id === "agentAgent") return null;
  const totals = unreadTotals(group.rows);
  const addLabel = group.id === "channels" ? t("actions.newChannel") : group.id === "dms" ? t("actions.newDm") : group.id === "agentDms" ? t("actions.newAgentDm") : null;
  return (
    <section className={cn("group/group mt-3 rounded-md", dropClass)} {...drop}>
      <div
        className={cn("flex h-7 items-center gap-1 pr-1 pl-1", section && human && "cursor-grab active:cursor-grabbing")}
        draggable={Boolean(section && human)}
        onDragStart={section && human ? (e) => dnd.start({ type: "section", id: section.id }, e) : undefined}
      >
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
        {section && human ? <SectionMenu section={section} sections={sections} /> : null}
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
            <Row key={r.channel.id} row={r} actors={actors} meId={meId} sections={sections} sectionId={group.custom ? group.id : null} human={human} />
          ))}
          {group.custom && group.rows.length === 0 ? <p className="px-2 py-1 text-meta text-fg-subtle">{t("sections.empty")}</p> : null}
        </div>
      )}
    </section>
  );
}

/** The conversation groups of the sidebar (round 31, part 4; sections: 34, part 2). */
export function ConversationList() {
  const { t } = useTranslation("conversation");
  const channels = useChannels();
  const sections = useSections();
  const actors = useActors();
  const session = useSession();
  const [creating, setCreating] = useState(false);
  const meId = session.data?.actor.id ?? "";
  // Sections are a human's own arrangement; the API refuses agents.
  const human = session.data?.actor.kind === "human";
  const groups = useMemo(
    () => (channels.data && actors.data ? buildSidebar(channels.data, sections.data ?? [], actors.data, meId) : []),
    [channels.data, sections.data, actors.data, meId],
  );
  if (!actors.data) return null;
  const list = sections.data ?? [];
  const fixed = groups.filter((g) => !g.custom);
  const ai = fixed.filter((g) => g.id === "agentDms" || g.id === "agentAgent");
  const props = { actors: actors.data, meId, sections: list, human };
  return (
    <nav aria-label={t("groups.channels")}>
      <DndProvider>
        {groups.filter((g) => g.custom).map((g) => (
          <Group key={g.id} group={g} {...props} />
        ))}
        {fixed.filter((g) => g.id === "channels" || g.id === "dms").map((g) => (
          <Group key={g.id} group={g} {...props} />
        ))}
        <p className="mt-4 px-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("groups.ai")}</p>
        {ai.map((g) => (
          <Group key={g.id} group={g} {...props} />
        ))}
      </DndProvider>
      {human ? (
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="mt-3 flex h-7 w-full items-center gap-1.5 rounded-md px-1 text-meta text-fg-subtle hover:bg-surface-sunken hover:text-fg"
        >
          <Plus aria-hidden className="size-3.5" />
          {t("sections.new")}
        </button>
      ) : null}
      {creating ? <SectionNameDialog onClose={() => setCreating(false)} /> : null}
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
