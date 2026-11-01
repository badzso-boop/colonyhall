// References in a message's text that the chat turns into chips
// (docs/handoff/34, part 6): Q-12 (a question), D-12 (a decision),
// [error:<id>] (a run error, details for humans) and [approval:<id>]
// (a tool permission request, shown as a card under the message).

export type ChipKind = "question" | "decision" | "error" | "approval";
export type Chip = { type: "chip"; kind: ChipKind; ref: string; raw: string };
export type ChipPart = { type: "text"; text: string } | Chip;

const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const pattern = () => new RegExp(`\\[(error|approval):(${UUID})\\]|(?<![A-Za-z0-9_-])([QD])-(\\d{1,9})(?![A-Za-z0-9_])`, "g");

/** Splits text into plain parts and chips, in order. Text without references is one part. */
export function splitChips(text: string): ChipPart[] {
  const out: ChipPart[] = [];
  let last = 0;
  for (const m of text.matchAll(pattern())) {
    const at = m.index;
    if (at > last) out.push({ type: "text", text: text.slice(last, at) });
    if (m[1]) out.push({ type: "chip", kind: m[1] as ChipKind, ref: m[2]!, raw: m[0] });
    else out.push({ type: "chip", kind: m[3] === "Q" ? "question" : "decision", ref: `${m[3]}-${m[4]}`, raw: m[0] });
    last = at + m[0].length;
  }
  if (last < text.length) out.push({ type: "text", text: text.slice(last) });
  return out;
}

/** The chips of one kind in a text, each once, in order (e.g. the approvals of a message). */
export function chipRefs(text: string, kind: ChipKind): string[] {
  const refs = splitChips(text).flatMap((p) => (p.type === "chip" && p.kind === kind ? [p.ref] : []));
  return [...new Set(refs)];
}
