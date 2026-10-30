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
