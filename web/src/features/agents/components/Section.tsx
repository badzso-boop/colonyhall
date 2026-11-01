import { useId, type ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { useLocalStorage } from "@/shared/hooks";
import { cn } from "@/shared/lib";

/**
 * A collapsible card of the overview. Its open state is a per-viewer
 * convenience (docs/08, 5.), remembered in localStorage per section id.
 */
export function Section({ id, title, aside, children, defaultOpen = true }: { id: string; title: string; aside?: ReactNode; children: ReactNode; defaultOpen?: boolean }) {
  const [open, setOpen] = useLocalStorage(`colony.agentOverview.${id}`, defaultOpen);
  const bodyId = useId();
  return (
    <section aria-label={title} className="rounded-lg border border-border bg-surface-raised">
      <h3 className="flex items-center gap-2 pr-3">
        <button
          type="button"
          aria-expanded={open}
          aria-controls={bodyId}
          onClick={() => setOpen(!open)}
          className="flex min-w-0 flex-1 items-center gap-2 px-3 py-2.5 text-left text-meta font-semibold tracking-wide text-fg-muted uppercase hover:text-fg"
        >
          <ChevronDown aria-hidden className={cn("size-4 shrink-0 transition-transform motion-reduce:transition-none", !open && "-rotate-90")} />
          <span className="truncate">{title}</span>
        </button>
        {aside}
      </h3>
      {open ? (
        <div id={bodyId} className="border-t border-border px-3 py-3 text-ui">
          {children}
        </div>
      ) : null}
    </section>
  );
}
