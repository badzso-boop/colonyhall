// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

/**
 * The pane manager (round 31, part 5): pure state and rules, no React.
 *
 * Beside the main column there are two slots, right and left (where the
 * sidebar normally is). A thread is the conversation pane; terminals, the
 * file browser and the agent overview are work panes, all in ONE work pane
 * with tabs.
 *
 *  1. A pane opens on the right when the right is empty.
 *  2. Thread on the right + a work pane opened → the work pane goes left
 *     (it replaces the sidebar); the thread stays.
 *  3. Work pane on the right + a thread opened → the thread takes the right,
 *     the work pane moves left with its state.
 *  4. One work pane, up to MAX_TABS tabs; a new tab beyond that closes the
 *     oldest one.
 *  5. A second thread replaces the first.
 *  6. (UI) While the left slot holds work, the sidebar opens as a drawer.
 *  7. Medium width: one slot; the most recently opened pane wins, the
 *     other is one tap away (focus).
 *  8. Narrow width: a navigation stack, conversation → thread → work.
 *  9. The state lives in the URL (toQuery / fromQuery).
 *
 * Rules 2 and 3 both mean: with a thread AND work open, work is left.
 */

export type WorkKind = "terminal" | "files" | "agent";
export type WorkTab = { kind: WorkKind; agentId: string; ref: string };
export type PaneState = { thread: string | null; tabs: WorkTab[]; active: number; focus: "thread" | "work" | null };
export type Width = "wide" | "medium" | "narrow";

export const MAX_TABS = 8;
export const empty: PaneState = { thread: null, tabs: [], active: 0, focus: null };

export const tabKey = (t: WorkTab) => `${t.kind}:${t.agentId}:${t.ref}`;

export type PaneAction =
  | { type: "openThread"; rootId: string }
  | { type: "closeThread" }
  | { type: "openWork"; tab: WorkTab }
  | { type: "selectTab"; index: number }
  | { type: "closeTab"; index: number }
  | { type: "closeWork" }
  | { type: "focus"; pane: "thread" | "work" };

export function reduce(s: PaneState, a: PaneAction): PaneState {
  switch (a.type) {
    case "openThread":
      return { ...s, thread: a.rootId, focus: "thread" };
    case "closeThread":
      return { ...s, thread: null, focus: s.tabs.length ? "work" : null };
    case "openWork": {
      const i = s.tabs.findIndex((t) => tabKey(t) === tabKey(a.tab));
      if (i >= 0) return { ...s, active: i, focus: "work" };
      let tabs = [...s.tabs, a.tab];
      if (tabs.length > MAX_TABS) tabs = tabs.slice(tabs.length - MAX_TABS);
      return { ...s, tabs, active: tabs.length - 1, focus: "work" };
    }
    case "selectTab":
      return a.index >= 0 && a.index < s.tabs.length ? { ...s, active: a.index, focus: "work" } : s;
    case "closeTab": {
      if (a.index < 0 || a.index >= s.tabs.length) return s;
      const tabs = s.tabs.filter((_, i) => i !== a.index);
      if (!tabs.length) return { ...s, tabs, active: 0, focus: s.thread ? "thread" : null };
      const active = a.index < s.active || s.active >= tabs.length ? Math.max(0, s.active - 1) : s.active;
      return { ...s, tabs, active };
    }
    case "closeWork":
      return { ...s, tabs: [], active: 0, focus: s.thread ? "thread" : null };
    case "focus":
      if (a.pane === "thread" && !s.thread) return s;
      if (a.pane === "work" && !s.tabs.length) return s;
      return { ...s, focus: a.pane };
  }
}

export type Placement = {
  /** What the left slot shows: the sidebar or the work pane. */
  left: "sidebar" | "work";
  /** What the right slot shows. */
  right: "thread" | "work" | null;
  /** Narrow: the one screen to show. */
  screen: "main" | "thread" | "work";
  /** Medium/narrow: the pane hidden behind the visible one, for the switcher. */
  other: "thread" | "work" | null;
};

export function place(s: PaneState, width: Width): Placement {
  const hasThread = s.thread !== null;
  const hasWork = s.tabs.length > 0;
  if (width === "wide") {
    return {
      left: hasThread && hasWork ? "work" : "sidebar",
      right: hasThread ? "thread" : hasWork ? "work" : null,
      screen: "main",
      other: null,
    };
  }
  const front = s.focus ?? (hasWork ? "work" : hasThread ? "thread" : null);
  const visible = front === "work" && hasWork ? "work" : hasThread ? "thread" : hasWork ? "work" : null;
  const other = visible === "work" && hasThread ? "thread" : visible === "thread" && hasWork ? "work" : null;
  if (width === "medium") return { left: "sidebar", right: visible, screen: "main", other };
  return { left: "sidebar", right: null, screen: visible ?? "main", other };
}

/* ---------- URL (rule 9): ?thread=<id>&work=kind:agent:ref,…&tab=<i>&focus=work ---------- */

const kinds: WorkKind[] = ["terminal", "files", "agent"];

export function toQuery(s: PaneState): Record<string, string> {
  const q: Record<string, string> = {};
  if (s.thread) q.thread = s.thread;
  if (s.tabs.length) {
    q.work = s.tabs.map((t) => [t.kind, t.agentId, t.ref].map(encodeURIComponent).join(":")).join(",");
    if (s.active) q.tab = String(s.active);
  }
  if (s.focus === "work" && s.thread) q.focus = "work";
  return q;
}

export function fromQuery(params: URLSearchParams): PaneState {
  const thread = params.get("thread") || null;
  const tabs: WorkTab[] = [];
  for (const part of (params.get("work") ?? "").split(",")) {
    const [kind, agentId, ref = ""] = part.split(":").map((x) => {
      try {
        return decodeURIComponent(x);
      } catch {
        return "";
      }
    });
    if (kind && kinds.includes(kind as WorkKind) && agentId) tabs.push({ kind: kind as WorkKind, agentId, ref });
  }
  const capped = tabs.slice(-MAX_TABS);
  const tab = Number(params.get("tab") ?? 0);
  const active = Number.isInteger(tab) && tab >= 0 && tab < capped.length ? tab : 0;
  const focus = params.get("focus") === "work" && capped.length ? "work" : thread ? "thread" : capped.length ? "work" : null;
  return { thread, tabs: capped, active, focus };
}
