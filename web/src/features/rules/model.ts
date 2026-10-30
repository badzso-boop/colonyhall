// The agent rules' placeholders (internal/modules/agentrules: Render). The
// preview fills them on the client; the server's text is authoritative.

export const MaxLen = 8000;
export const placeholders = ["handle", "admins", "workspace"] as const;
export type Placeholder = (typeof placeholders)[number];

/** Mirrors the server's Render: handle without @, admins as "@a, @b", unknown {x} kept. */
export function renderRules(text: string, handle: string, adminHandles: string[], workspace: string): string {
  const admins = adminHandles
    .filter(Boolean)
    .map((a) => (a.startsWith("@") ? a : `@${a}`))
    .join(", ");
  return text.replaceAll("{handle}", handle.replace(/^@/, "")).replaceAll("{admins}", admins).replaceAll("{workspace}", workspace);
}

export type Segment = { text: string } | { placeholder: Placeholder };

/** The text split around the known placeholders, for showing them as chips. */
export function segments(text: string): Segment[] {
  const out: Segment[] = [];
  const re = /\{(handle|admins|workspace)\}/g;
  let last = 0;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push({ text: text.slice(last, m.index) });
    out.push({ placeholder: m[1] as Placeholder });
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last) });
  return out;
}

/** "normal" below 90 % of the limit, "near" from 90 %, "over" above it (in characters, as the server counts). */
export function lengthState(text: string): "normal" | "near" | "over" {
  const n = [...text].length;
  return n > MaxLen ? "over" : n >= MaxLen * 0.9 ? "near" : "normal";
}
