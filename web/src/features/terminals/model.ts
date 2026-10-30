import type { Terminal } from "./api";

/** The status badge: its i18n key, the exit code and the tone. */
export function terminalBadge(t: Terminal): { key: string; code: number | null; tone: "warning" | "success" | "danger" | "neutral" } {
  switch (t.status) {
    case "running":
      return { key: "status.running", code: null, tone: "warning" };
    case "finished":
      return { key: t.exit_code && t.exit_code !== 0 ? "status.error" : "status.finished", code: t.exit_code ?? 0, tone: t.exit_code ? "danger" : "success" };
    case "error":
      return { key: "status.error", code: t.exit_code ?? 1, tone: "danger" };
    case "killed":
      return { key: "status.killed", code: null, tone: "neutral" };
  }
}

/** "43:12" or "1:02:05". */
export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

/** The read-only tmux attach line shown for copying (round 23). */
export function sshAttach(handle: string, name: string): string {
  return `tmux -L colonyhall attach -r -t colony-${handle}-${name}`;
}
