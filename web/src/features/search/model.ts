import type { Schemas } from "@/shared/api";

export type SearchHit = Schemas["SearchHit"];
export type SearchScores = Schemas["SearchScores"];
export type SearchResponse = Schemas["SearchResponse"];
export type SearchKind = Schemas["SearchSourceKind"];
export type SearchMode = SearchResponse["mode"];

export const searchModes: readonly SearchMode[] = ["keyword", "semantic", "hybrid"];
export const DEFAULT_MODE: SearchMode = "hybrid";
/** The order of the result groups. */
export const searchKinds: readonly SearchKind[] = ["message", "wiki", "decision", "summary"];

export function isSearchMode(v: unknown): v is SearchMode {
  return typeof v === "string" && (searchModes as readonly string[]).includes(v);
}

/** The search page's URL for a query (the command palette opens it). The default mode stays out of the URL. */
export function searchPath(q: string, mode: SearchMode = DEFAULT_MODE): string {
  const s = searchParams(q, mode).toString();
  return s ? `/search?${s}` : "/search";
}

/** The search page's URL parameters: the trimmed query, and the mode unless it is the default. */
export function searchParams(q: string, mode: SearchMode = DEFAULT_MODE): URLSearchParams {
  const p = new URLSearchParams();
  const query = q.trim();
  if (query) p.set("q", query);
  if (mode !== DEFAULT_MODE) p.set("mode", mode);
  return p;
}

export type SnippetPart = { text: string; mark: boolean };

/**
 * Splits a snippet into plain and matched parts: the server wraps keyword
 * matches in « and ». An unbalanced « (no closing ») and a stray » stay
 * as plain text, so nothing of the snippet is ever lost.
 */
export function snippetParts(snippet: string): SnippetPart[] {
  const out: SnippetPart[] = [];
  const push = (text: string, mark: boolean) => {
    if (!text) return;
    const last = out[out.length - 1];
    if (last && last.mark === mark) last.text += text;
    else out.push({ text, mark });
  };
  let at = 0;
  while (at < snippet.length) {
    const open = snippet.indexOf("«", at);
    if (open < 0) break;
    const close = snippet.indexOf("»", open + 1);
    if (close < 0) break;
    push(snippet.slice(at, open), false);
    const inner = snippet.slice(open + 1, close);
    // «» with nothing inside is not a match.
    if (inner) push(inner, true);
    at = close + 1;
  }
  push(snippet.slice(at), false);
  return out;
}

export type HitGroup = { kind: SearchKind; hits: SearchHit[] };

/** The hits grouped by source kind, in the fixed group order; the server's ranking is kept within a group. */
export function groupHits(hits: readonly SearchHit[]): HitGroup[] {
  return searchKinds
    .map((kind) => ({ kind, hits: hits.filter((h) => h.kind === kind) }))
    .filter((g) => g.hits.length > 0);
}

/** Where a hit leads in the app; null when the hit lacks what its route needs. */
export function hitHref(hit: SearchHit): string | null {
  switch (hit.kind) {
    case "message":
    case "summary": {
      if (!hit.channel_id) return null;
      const thread = hit.thread_root_id ? `?thread=${encodeURIComponent(hit.thread_root_id)}` : "";
      return `/c/${encodeURIComponent(hit.channel_id)}${thread}`;
    }
    case "wiki":
      return hit.ref ? `/wiki/${encodeURIComponent(hit.ref)}` : "/wiki";
    case "decision":
      return hit.ref ? `/decisions#${encodeURIComponent(hit.ref)}` : "/decisions";
  }
}

export type ScorePart = { stage: "keyword" | "semantic" | "fused" | "rerank"; value: number };

/** The stage scores in pipeline order, only the stages that saw the hit; empty without scores (non-admins). */
export function scoreParts(scores: SearchScores | undefined): ScorePart[] {
  if (!scores) return [];
  const out: ScorePart[] = [];
  if (scores.keyword !== undefined) out.push({ stage: "keyword", value: scores.keyword });
  if (scores.semantic !== undefined) out.push({ stage: "semantic", value: scores.semantic });
  out.push({ stage: "fused", value: scores.fused });
  if (scores.rerank !== undefined) out.push({ stage: "rerank", value: scores.rerank });
  return out;
}

/** A score shown compactly: two significant digits, at most four decimals (0.81, 0.032). */
export function formatScore(v: number): string {
  if (v === 0) return "0";
  const digits = Math.min(4, Math.max(2, 1 - Math.floor(Math.log10(Math.abs(v)))));
  return v.toFixed(digits).replace(/(\.\d*?)0+$/, "$1").replace(/\.$/, "");
}
