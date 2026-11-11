import { type KeyboardEvent, type PointerEvent as RPointerEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { CalendarDays, Circle, CircleDashed, Contrast, ListChecks, Maximize2, Minus, NotebookPen, X } from "lucide-react";
import { cn } from "@/shared/lib";
import { useWidth } from "@/shared/hooks";
import { Sheet } from "@/shared/ui";
import { BUBBLE, KINDS, TITLE, dispatchWindow, stack, useWindows, type Rect, type WindowKind } from "@/shared/windows";
import { CalendarWindow, TasksWindow, useWindowBadge } from "@/features/planner";
import { NotesWindow, useNotesBadge } from "@/features/notes";

export const windowMeta: Record<WindowKind, { icon: typeof ListChecks; route: string; Content: () => ReactNode }> = {
  tasks: { icon: ListChecks, route: "/tasks", Content: TasksWindow },
  calendar: { icon: CalendarDays, route: "/calendar", Content: CalendarWindow },
  notes: { icon: NotebookPen, route: "/notes", Content: NotesWindow },
};

const opacityIcon = { normal: Circle, auto: Contrast, dim: CircleDashed } as const;
const STEP = 16;
type Edge = "n" | "s" | "e" | "w" | "ne" | "nw" | "se" | "sw";
const edges: Array<{ edge: Edge; className: string }> = [
  { edge: "n", className: "inset-x-2 -top-1 h-2 cursor-ns-resize" },
  { edge: "s", className: "inset-x-2 -bottom-1 h-2 cursor-ns-resize" },
  { edge: "e", className: "inset-y-2 -right-1 w-2 cursor-ew-resize" },
  { edge: "w", className: "inset-y-2 -left-1 w-2 cursor-ew-resize" },
  { edge: "ne", className: "-top-1 -right-1 size-3 cursor-nesw-resize" },
  { edge: "nw", className: "-top-1 -left-1 size-3 cursor-nwse-resize" },
  { edge: "se", className: "-right-1 -bottom-1 size-3 cursor-nwse-resize" },
  { edge: "sw", className: "-bottom-1 -left-1 size-3 cursor-nesw-resize" },
];

function resized(r: Rect, edge: Edge, dx: number, dy: number): Rect {
  let { x, y, w, h } = r;
  if (edge.includes("e")) w += dx;
  if (edge.includes("s")) h += dy;
  if (edge.includes("w")) {
    x += dx;
    w -= dx;
  }
  if (edge.includes("n")) {
    y += dy;
    h -= dy;
  }
  return { x, y, w, h };
}

/** Drags with the pointer captured; calls onMove with the offset from the start. */
function startDrag(e: RPointerEvent, onMove: (dx: number, dy: number) => void, onEnd?: (moved: boolean) => void) {
  const sx = e.clientX;
  const sy = e.clientY;
  let moved = false;
  const target = e.currentTarget as HTMLElement;
  target.setPointerCapture(e.pointerId);
  const move = (ev: PointerEvent) => {
    const dx = ev.clientX - sx;
    const dy = ev.clientY - sy;
    if (Math.abs(dx) + Math.abs(dy) > 3) moved = true;
    onMove(dx, dy);
  };
  const up = () => {
    target.removeEventListener("pointermove", move);
    target.removeEventListener("pointerup", up);
    target.removeEventListener("pointercancel", up);
    onEnd?.(moved);
  };
  target.addEventListener("pointermove", move);
  target.addEventListener("pointerup", up);
  target.addEventListener("pointercancel", up);
}

function FloatingWindow({ kind }: { kind: WindowKind }) {
  const { t } = useTranslation("layout");
  const navigate = useNavigate();
  const win = useWindows((s) => s.wins[kind]);
  const { icon: Icon, route, Content } = windowMeta[kind];
  const OpacityIcon = opacityIcon[win.opacity];
  const titleId = `fw-${kind}-title`;
  const name = t(`windows.${kind}`);

  const onTitleDown = (e: RPointerEvent) => {
    if ((e.target as HTMLElement).closest("button")) return;
    dispatchWindow({ type: "focus", kind });
    const start = win.rect;
    startDrag(e, (dx, dy) => dispatchWindow({ type: "move", kind, x: start.x + dx, y: start.y + dy }));
  };
  const onTitleKey = (e: KeyboardEvent) => {
    const d = { ArrowLeft: [-STEP, 0], ArrowRight: [STEP, 0], ArrowUp: [0, -STEP], ArrowDown: [0, STEP] }[e.key];
    if (!d || !e.altKey) return;
    e.preventDefault();
    const r = win.rect;
    if (e.shiftKey) dispatchWindow({ type: "resize", kind, rect: { ...r, w: r.w + d[0]!, h: r.h + d[1]! } });
    else dispatchWindow({ type: "move", kind, x: r.x + d[0]!, y: r.y + d[1]! });
  };

  return (
    <section
      role="dialog"
      aria-modal="false"
      aria-labelledby={titleId}
      onPointerDownCapture={() => dispatchWindow({ type: "focus", kind })}
      onKeyDown={(e) => {
        // Esc gives the focus back to the page; the window stays.
        if (e.key === "Escape") document.getElementById("main")?.focus();
      }}
      style={{ left: win.rect.x, top: win.rect.y, width: win.rect.w, height: win.rect.h, zIndex: `calc(var(--z-window) + ${win.z})` }}
      className={cn(
        "fixed flex flex-col overflow-visible rounded-lg border border-border-strong bg-surface-raised shadow-overlay transition-opacity duration-200 motion-reduce:transition-none",
        win.opacity === "auto" && "[&:not(:hover):not(:focus-within)]:opacity-55",
        win.opacity === "dim" && "[&:not(:hover):not(:focus-within)]:opacity-20",
      )}
    >
      <div
        tabIndex={0}
        title={t("windows.move")}
        onPointerDown={onTitleDown}
        onKeyDown={onTitleKey}
        onDoubleClick={(e) => {
          if ((e.target as HTMLElement).closest("button")) return;
          navigate(route);
          dispatchWindow({ type: "close", kind });
        }}
        className="flex shrink-0 cursor-move items-center gap-2 rounded-t-lg border-b border-border bg-surface-sunken pr-1 pl-2.5 text-ui font-semibold select-none"
        style={{ height: TITLE }}
      >
        <Icon aria-hidden className="size-4 text-fg-subtle" />
        <span id={titleId} className="flex-1 truncate">
          {name}
        </span>
        {(
          [
            ["opacity", t(`windows.opacity.${win.opacity}`), <OpacityIcon key="o" className="size-4" />, () => dispatchWindow({ type: "cycleOpacity", kind }), win.opacity !== "normal"],
            ["min", t("windows.minimize"), <Minus key="m" className="size-4" />, () => dispatchWindow({ type: "minimize", kind }), false],
            [
              "full",
              t("windows.fullPage"),
              <Maximize2 key="f" className="size-3.5" />,
              () => {
                navigate(route);
                dispatchWindow({ type: "close", kind });
              },
              false,
            ],
          ] as const
        ).map(([key, label, icon, run, on]) => (
          <button
            key={key}
            type="button"
            aria-label={label}
            title={label}
            onClick={run}
            className={cn("grid h-6.5 w-7 place-items-center rounded-sm text-fg-muted hover:bg-surface hover:text-fg", on && "text-accent-text")}
          >
            {icon}
          </button>
        ))}
        <button
          type="button"
          aria-label={t("windows.close")}
          title={t("windows.close")}
          onClick={() => dispatchWindow({ type: "close", kind })}
          className="grid h-6.5 w-7 place-items-center rounded-sm text-fg-muted hover:bg-accent hover:text-accent-fg"
        >
          <X className="size-4" />
        </button>
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <Content />
      </div>
      {edges.map(({ edge, className }) => (
        <div
          key={edge}
          aria-hidden
          className={cn("absolute touch-none", className)}
          onPointerDown={(e) => {
            e.stopPropagation();
            const start = win.rect;
            startDrag(e, (dx, dy) => dispatchWindow({ type: "resize", kind, rect: resized(start, edge, dx, dy) }));
          }}
        />
      ))}
    </section>
  );
}

function Bubble({ kind }: { kind: WindowKind }) {
  const { t } = useTranslation("layout");
  const win = useWindows((s) => s.wins[kind]);
  const plannerBadge = useWindowBadge(kind);
  const notesBadge = useNotesBadge();
  const badge = kind === "notes" ? notesBadge : plannerBadge;
  const Icon = windowMeta[kind].icon;
  return (
    <button
      type="button"
      aria-label={t("windows.restore", { name: t(`windows.${kind}`) })}
      title={t(`windows.${kind}`)}
      style={{ left: win.bubble.x, top: win.bubble.y, width: BUBBLE, height: BUBBLE, zIndex: "var(--z-window)" }}
      onPointerDown={(e) => {
        const start = win.bubble;
        startDrag(
          e,
          (dx, dy) => dispatchWindow({ type: "moveBubble", kind, x: start.x + dx, y: start.y + dy }),
          (moved) => {
            if (!moved) dispatchWindow({ type: "restore", kind });
          },
        );
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          dispatchWindow({ type: "restore", kind });
        }
      }}
      className={cn(
        "fixed grid touch-none place-items-center rounded-full border border-border-strong bg-surface-raised text-fg shadow-overlay transition-opacity duration-200 motion-reduce:transition-none",
        badge.news && "ring-2 ring-accent",
        win.opacity === "auto" && "[&:not(:hover):not(:focus-visible)]:opacity-55",
        win.opacity === "dim" && "[&:not(:hover):not(:focus-visible)]:opacity-20",
      )}
    >
      <Icon aria-hidden className="size-5" />
      {badge.text ? (
        <span className="tabular absolute -top-1 -right-1.5 rounded-full bg-accent px-1.5 text-[10px] leading-4 font-semibold text-accent-fg">{badge.text}</span>
      ) : null}
    </button>
  );
}

/**
 * The floating windows and the minimised circles (round 32, part 1).
 * Phones have no floating windows: an open one shows as a bottom Sheet.
 */
export function FloatingWindows() {
  const { t } = useTranslation("layout");
  const width = useWidth();
  const wins = useWindows((s) => s.wins);
  const open = stack(wins);
  if (width === "narrow") {
    const front = open[open.length - 1];
    if (!front) return null;
    const { Content } = windowMeta[front];
    return (
      <Sheet open onOpenChange={(o) => !o && dispatchWindow({ type: "close", kind: front })} title={t(`windows.${front}`)} closeLabel={t("windows.close")}>
        <Content />
      </Sheet>
    );
  }
  return (
    <>
      {/* A fixed DOM order: z-index stacks them. Reordering the nodes on focus
          would make the browser drop the click that focused the window. */}
      {KINDS.filter((k) => wins[k].state === "open").map((k) => (
        <FloatingWindow key={k} kind={k} />
      ))}
      {KINDS.filter((k) => wins[k].state === "min").map((k) => (
        <Bubble key={k} kind={k} />
      ))}
    </>
  );
}
