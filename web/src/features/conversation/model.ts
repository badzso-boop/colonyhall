import type { Schemas } from "@/shared/api";
import { rowBadge } from "./channel";

export type Channel = Schemas["Channel"];
export type Actor = Schemas["Actor"];
export type Section = Schemas["SidebarSection"];

/**
 * Which sidebar group a conversation belongs to (round 31, part 4):
 * channel (public or private), a DM with humans only, a DM with at least
 * one agent, an agent–agent DM (the viewer is not in it: admins see these),
 * or a note to self.
 */
export type ConversationKind = "channel" | "human-dm" | "agent-dm" | "agent-agent" | "self";

export function classifyConversation(ch: Channel, actors: Map<string, Actor>, meId: string): ConversationKind {
  if (ch.kind !== "dm") return "channel";
  const members = ch.member_ids ?? [];
  const known = members.map((id) => actors.get(id)).filter((a): a is Actor => a !== undefined);
  if (!members.includes(meId)) {
    // Someone else's DM, visible to admins: agent–agent when an agent is in it.
    return known.some((a) => a.kind === "agent") ? "agent-agent" : "human-dm";
  }
  const others = known.filter((a) => a.id !== meId);
  if (members.every((id) => id === meId)) return "self";
  if (others.some((a) => a.kind === "agent")) return "agent-dm";
  // Only humans, or members no longer listed: a human DM.
  return "human-dm";
}

/** The other members of a DM (the viewer left out), known actors only. */
export function dmPeers(ch: Channel, actors: Map<string, Actor>, meId: string): Actor[] {
  return (ch.member_ids ?? []).filter((id) => id !== meId).map((id) => actors.get(id)).filter((a): a is Actor => a !== undefined);
}

/** The label of a conversation: "#name" for channels, the peers' names for DMs. */
export function conversationTitle(ch: Channel, actors: Map<string, Actor>, meId: string): string {
  if (ch.kind !== "dm") return ch.name;
  const peers = dmPeers(ch, actors, meId);
  // The viewer's own DM (reminders arrive there, round 55): their own name.
  if (peers.length === 0) {
    const me = (ch.member_ids ?? []).every((id) => id === meId) ? actors.get(meId) : undefined;
    return me ? me.display_name || me.handle : ch.name;
  }
  const agentAgent = !(ch.member_ids ?? []).includes(meId);
  return peers.map((a) => (a.kind === "agent" || agentAgent ? `@${a.handle}` : a.display_name || a.handle)).join(agentAgent ? " ↔ " : ", ");
}

export type { SidebarRow as ConversationRow };
export type SidebarRow = { channel: Channel; kind: ConversationKind; title: string; observed: boolean };
export type SidebarGroup = { id: string; title: string | null; custom: boolean; collapsed: boolean; rows: SidebarRow[] };

/**
 * The sidebar, top to bottom: the user's own sections (in their order), then
 * Csatornák, Közvetlen üzenetek, AI-beszélgetések (Te és az agentek, Agentek
 * egymás közt). A conversation placed in a section is not repeated below.
 * Group titles of the fixed groups are i18n keys, filled in by the UI.
 */
export function buildSidebar(channels: Channel[], sections: Section[], actors: Map<string, Actor>, meId: string): SidebarGroup[] {
  const byId = new Map(channels.map((c) => [c.id, c]));
  const row = (c: Channel): SidebarRow => ({
    channel: c,
    kind: classifyConversation(c, actors, meId),
    title: conversationTitle(c, actors, meId),
    observed: !c.is_member,
  });
  const placed = new Set<string>();
  const out: SidebarGroup[] = [];
  for (const s of [...sections].sort((a, b) => a.position - b.position)) {
    const rows = [...s.items]
      .sort((a, b) => a.position - b.position)
      .map((i) => byId.get(i.channel_id))
      .filter((c): c is Channel => c !== undefined)
      .map(row);
    rows.forEach((r) => placed.add(r.channel.id));
    out.push({ id: s.id, title: s.name, custom: true, collapsed: s.collapsed, rows });
  }
  const rest = channels.filter((c) => !placed.has(c.id)).map(row);
  const byTitle = (a: SidebarRow, b: SidebarRow) => a.title.localeCompare(b.title, "hu");
  const fixed: Array<[string, (r: SidebarRow) => boolean]> = [
    ["channels", (r) => r.kind === "channel"],
    ["dms", (r) => r.kind === "human-dm" || r.kind === "self"],
    ["agentDms", (r) => r.kind === "agent-dm"],
    ["agentAgent", (r) => r.kind === "agent-agent"],
  ];
  for (const [id, match] of fixed) {
    out.push({ id, title: null, custom: false, collapsed: false, rows: rest.filter(match).sort(byTitle) });
  }
  return out;
}

/**
 * Unread rows count once each; mentions add up. Used for collapsed groups and
 * the tab title. A muted conversation's unread messages do not count; its
 * mentions of the viewer still do (round 34).
 */
export function unreadTotals(rows: SidebarRow[], now = new Date()): { unread: number; mentions: number } {
  let unread = 0;
  let mentions = 0;
  for (const r of rows) {
    const b = rowBadge(r.channel, now);
    if (b.bold) unread++;
    mentions += b.count;
  }
  return { unread, mentions };
}

type Msg = Schemas["Message"];

/**
 * A reply arrived: bump its root's summary in a list of messages (count,
 * last reply time, the replier first among the participants, at most 3).
 */
export function applyReply(list: Msg[], reply: Msg): Msg[] {
  return list.map((m) => {
    if (m.id !== reply.thread_root_id) return m;
    const others = (m.reply_participant_ids ?? []).filter((id) => id !== reply.author_id);
    return {
      ...m,
      reply_count: m.reply_count + 1,
      last_reply_at: reply.created_at,
      reply_participant_ids: [reply.author_id, ...others].slice(0, 3),
    };
  });
}

/** The main stream of a channel shows root messages only; replies live in threads. */
export function mainStream(messages: Msg[], kind: Channel["kind"]): Msg[] {
  return kind === "dm" ? messages : messages.filter((m) => !m.thread_root_id);
}
