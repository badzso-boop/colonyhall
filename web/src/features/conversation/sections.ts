import type { Section } from "./model";

/**
 * Sidebar sections (docs/handoff/34, part 2): pure moves over the viewer's
 * sections, used for the optimistic update and to compute what the server
 * is told. Positions are renumbered 0..n-1 after every move.
 */

const sorted = <T extends { position: number }>(xs: T[]): T[] => [...xs].sort((a, b) => a.position - b.position);

const renumber = <T extends { position: number }>(xs: T[]): T[] => xs.map((x, i) => (x.position === i ? x : { ...x, position: i }));

/** The section a conversation is placed in, or null when it sits in its default group. */
export function sectionOf(sections: Section[], channelId: string): string | null {
  return sections.find((s) => s.items.some((i) => i.channel_id === channelId))?.id ?? null;
}

/** The conversations of a section, in order. */
export function itemOrder(sections: Section[], sectionId: string): string[] {
  const s = sections.find((x) => x.id === sectionId);
  return s ? sorted(s.items).map((i) => i.channel_id) : [];
}

/** The section ids, in order. */
export function sectionOrder(sections: Section[]): string[] {
  return sorted(sections).map((s) => s.id);
}

/** Moves a section to `toIndex` (clamped) among the sections. */
export function moveSection(sections: Section[], sectionId: string, toIndex: number): Section[] {
  const list = sorted(sections);
  const from = list.findIndex((s) => s.id === sectionId);
  if (from < 0) return sections;
  const [moved] = list.splice(from, 1);
  list.splice(Math.max(0, Math.min(toIndex, list.length)), 0, moved!);
  return renumber(list);
}

/**
 * Moves a conversation into `target` at `index` (the end when omitted), or
 * back to its default group when `target` is null. It is taken out of every
 * other section first: a conversation is in at most one section.
 */
export function moveConversation(sections: Section[], channelId: string, target: string | null, index?: number): Section[] {
  const out = sorted(sections).map((s) => {
    const had = s.items.some((i) => i.channel_id === channelId);
    let items = had ? sorted(s.items).filter((i) => i.channel_id !== channelId) : sorted(s.items);
    if (s.id === target) {
      const at = index === undefined ? items.length : Math.max(0, Math.min(index, items.length));
      items = [...items.slice(0, at), { channel_id: channelId, position: at }, ...items.slice(at)];
    } else if (!had) {
      return s;
    }
    return { ...s, items: renumber(items) };
  });
  return out;
}

/**
 * Where a dragged conversation lands when dropped on `targetId` in `order`
 * (that section's conversations): at the target's index. Coming from
 * elsewhere that is just before the target; moving down within the same
 * section it is just after it (the list closes up once the row is lifted).
 */
export function dropIndex(order: string[], draggedId: string, targetId: string): number {
  const at = order.indexOf(targetId);
  return at < 0 ? order.filter((id) => id !== draggedId).length : at;
}

/** Inserts a new section (from the server) or replaces one with the same id. */
export function upsertSection(sections: Section[], section: Section): Section[] {
  return sections.some((s) => s.id === section.id) ? sections.map((s) => (s.id === section.id ? section : s)) : [...sections, section];
}

/** A section name as the server takes it: trimmed, 1–40 bytes of UTF-8 (the server counts bytes, so "á" is two). */
export function validSectionName(name: string): boolean {
  const n = name.trim();
  return n.length > 0 && new TextEncoder().encode(n).length <= 40;
}
