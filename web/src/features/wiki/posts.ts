// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { Schemas } from "@/shared/api";

export { lineDiff, type DiffLine } from "@/shared/lib";

// A wiki page read as a forum topic (round 32, part 7): the first
// revision is the opening post and every append a reply. The server only adds
// to the end on append, so a reply is the revision's body minus the previous.

export type Revision = Schemas["WikiRevision"];

export type Post =
  | { kind: "post"; rev: number; authorId: string; at: string; body: string; opening: boolean }
  /** A human rewrite; `body` is set when it replaced the text in a way we cannot split into posts. */
  | { kind: "edited"; rev: number; authorId: string; at: string; body?: string }
  | { kind: "deleted" | "restored"; rev: number; authorId: string; at: string };

// The header the server writes before each append: "\n\n---\n*@handle, 2026-10-30 09:15:*\n\n".
const appendHeader = /^\s*---\n\*@[^,\n]+, [^\n]*?:\*\n+/;

export function wikiPosts(revisions: Revision[]): Post[] {
  const sorted = [...revisions].sort((a, b) => a.rev - b.rev);
  const out: Post[] = [];
  let prev = "";
  for (const r of sorted) {
    const base = { rev: r.rev, authorId: r.author_id, at: r.created_at };
    switch (r.kind) {
      case "create":
        out.push({ kind: "post", ...base, body: r.body, opening: true });
        break;
      case "append":
        if (r.body.startsWith(prev)) out.push({ kind: "post", ...base, body: r.body.slice(prev.length).replace(appendHeader, "").trim(), opening: false });
        else out.push({ kind: "edited", ...base, body: r.body });
        break;
      case "edit":
        out.push({ kind: "edited", ...base });
        break;
      case "delete":
      case "restore":
        out.push({ kind: r.kind === "delete" ? "deleted" : "restored", ...base });
        // These keep the text; an empty body is not a new base.
        if (!r.body) continue;
        break;
    }
    prev = r.body;
  }
  return out;
}

/** Tags with counts, most used first, then by name. */
export function tagCounts(pages: { tags: string[] }[]): [string, number][] {
  const m = new Map<string, number>();
  for (const p of pages) for (const t of p.tags) m.set(t, (m.get(t) ?? 0) + 1);
  return [...m.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
}

/** "> " before every line, and the author, for the reply box. */
export function quote(body: string, handle: string): string {
  return `> @${handle}:\n${body
    .split("\n")
    .map((l) => `> ${l}`)
    .join("\n")}\n\n`;
}
