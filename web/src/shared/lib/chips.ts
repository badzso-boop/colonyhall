// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// References in a message's text that the chat turns into chips
// (round 34, part 6): Q-12 (a question), D-12 (a decision),
// [error:<id>] (a run error, details for humans) and [approval:<id>]
// (a tool permission request, shown as a card under the message).
// Round 55: [message:<channel>/<message>] or [message:<channel>/<message>/<thread root>]
// (a link to a message: reminders, forwards).

export type ChipKind = "question" | "decision" | "error" | "approval" | "message";
export type Chip = { type: "chip"; kind: ChipKind; ref: string; raw: string };
export type ChipPart = { type: "text"; text: string } | Chip;

const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const pattern = () =>
  new RegExp(`\\[(error|approval):(${UUID})\\]|\\[message:(${UUID}/${UUID}(?:/${UUID})?)\\]|(?<![A-Za-z0-9_-])([QD])-(\\d{1,9})(?![A-Za-z0-9_])`, "g");

/** Splits text into plain parts and chips, in order. Text without references is one part. */
export function splitChips(text: string): ChipPart[] {
  const out: ChipPart[] = [];
  let last = 0;
  for (const m of text.matchAll(pattern())) {
    const at = m.index;
    if (at > last) out.push({ type: "text", text: text.slice(last, at) });
    if (m[1]) out.push({ type: "chip", kind: m[1] as ChipKind, ref: m[2]!, raw: m[0] });
    else if (m[3]) out.push({ type: "chip", kind: "message", ref: m[3], raw: m[0] });
    else out.push({ type: "chip", kind: m[4] === "Q" ? "question" : "decision", ref: `${m[4]}-${m[5]}`, raw: m[0] });
    last = at + m[0].length;
  }
  if (last < text.length) out.push({ type: "text", text: text.slice(last) });
  return out;
}

/** A message chip's reference as a message link target. */
export function messageChipRef(ref: string): { channel_id: string; id: string; thread_root_id?: string } {
  const [channel_id = "", id = "", thread_root_id] = ref.split("/");
  return { channel_id, id, ...(thread_root_id ? { thread_root_id } : {}) };
}

/** The chips of one kind in a text, each once, in order (e.g. the approvals of a message). */
export function chipRefs(text: string, kind: ChipKind): string[] {
  const refs = splitChips(text).flatMap((p) => (p.type === "chip" && p.kind === kind ? [p.ref] : []));
  return [...new Set(refs)];
}
