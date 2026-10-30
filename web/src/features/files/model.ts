import type { Schemas } from "@/shared/api";

export type GitLetter = "M" | "A" | "D" | "?" | "R";

/** git status codes ("M", " M", "??", "A", "R", "D" …) → the badge letter. */
export function gitLetter(status: string): GitLetter | null {
  const s = status.trim();
  if (s === "??" || s === "?") return "?";
  if (s.includes("R")) return "R";
  if (s.includes("A")) return "A";
  if (s.includes("D")) return "D";
  if (s.includes("M") || s.includes("U")) return "M";
  return null;
}

/** Per-path letters, and every ancestor folder of a changed file (the dot on folders). */
export function gitMarks(files: Schemas["FsGitFile"][]): { letters: Map<string, GitLetter>; dirty: Set<string> } {
  const letters = new Map<string, GitLetter>();
  const dirty = new Set<string>();
  for (const f of files) {
    const l = gitLetter(f.status);
    if (!l) continue;
    const path = f.path.replace(/\/$/, "");
    letters.set(path, l);
    const parts = path.split("/");
    for (let i = 1; i < parts.length; i++) dirty.add(parts.slice(0, i).join("/"));
  }
  return { letters, dirty };
}

const languages: Record<string, string> = {
  go: "go", ts: "typescript", tsx: "typescript", js: "javascript", mjs: "javascript", jsx: "javascript", json: "json",
  md: "markdown", yaml: "yaml", yml: "yaml", sql: "sql", sh: "shell", bash: "shell", py: "python", rs: "rust",
  html: "html", css: "css", toml: "ini", ini: "ini", dockerfile: "dockerfile", xml: "xml", java: "java", c: "c", h: "c", cpp: "cpp",
};

/** The Monaco language of a file name. */
export function languageOf(path: string): string {
  const name = path.split("/").pop()?.toLowerCase() ?? "";
  if (name === "dockerfile") return "dockerfile";
  if (name === "makefile") return "makefile";
  return languages[name.split(".").pop() ?? ""] ?? "plaintext";
}

/** "src/main.go:12-20" → the path and the selected lines (a work tab's ref). */
export function parseRef(ref: string): { path: string; start?: number; end?: number } {
  const m = /^(.*?):(\d+)(?:-(\d+))?$/.exec(ref);
  if (!m) return { path: ref };
  const start = Number(m[2]);
  return { path: m[1]!, start, end: m[3] ? Number(m[3]) : start };
}

/** The chat reference of a selection: "@handle:path:12-20" (round 20). */
export function lineReference(handle: string, path: string, start: number, end: number): string {
  return `@${handle}:${path}:${start === end ? start : `${start}-${end}`}`;
}

export const LARGE_FILE = 2 * 1024 * 1024;
export const RANGE = 1024 * 1024;

/** The Range header for a large file's head or tail. */
export function rangeFor(size: number, part: "start" | "end"): string {
  return part === "start" ? `bytes=0-${RANGE - 1}` : `bytes=${Math.max(0, size - RANGE)}-${size - 1}`;
}
