// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { ReactNode } from "react";
import { cn } from "@/shared/lib";

/** What a list or pane shows when it has nothing, with an optional action. */
export function EmptyState({ icon, title, body, action }: { icon?: ReactNode; title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 px-6 py-10 text-center">
      {icon ? <div className="text-fg-subtle">{icon}</div> : null}
      <p className="text-title font-semibold">{title}</p>
      {body ? <p className="max-w-sm text-ui text-fg-muted">{body}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

/** A loading placeholder in the shape of the content to come. */
export function Skeleton({ className }: { className?: string }) {
  return <span aria-hidden className={cn("block rounded-md bg-surface-sunken motion-safe:animate-pulse", className ?? "h-4 w-full")} />;
}

/** Keyboard key, as in the shortcut help. */
export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-block rounded-sm border border-b-2 border-border-strong bg-surface px-1.5 font-mono text-meta leading-5 text-fg-muted">
      {children}
    </kbd>
  );
}

const noticeTone = {
  info: "border-info/40 bg-info-soft text-info",
  warning: "border-warning/40 bg-warning-soft text-warning",
  danger: "border-danger/40 bg-danger-soft text-danger",
  success: "border-success/40 bg-success-soft text-success",
} as const;

/** A persistent banner inside a page or dialog (e.g. the immutable-configuration warning). */
export function Notice({ tone = "info", icon, children, action }: { tone?: keyof typeof noticeTone; icon?: ReactNode; children: ReactNode; action?: ReactNode }) {
  return (
    <div role={tone === "danger" ? "alert" : "note"} className={cn("flex items-start gap-2 rounded-md border px-3 py-2 text-ui", noticeTone[tone])}>
      {icon ? <span className="mt-0.5 shrink-0">{icon}</span> : null}
      <div className="min-w-0 flex-1 text-fg">{children}</div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
