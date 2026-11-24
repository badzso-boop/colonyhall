// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useRef, type KeyboardEvent, type PointerEvent as RPointerEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useLocalStorage } from "@/shared/hooks";
import { cn } from "@/shared/lib";
import { MIN_SLOT, clampSlot, defaultSlotWidth, type SlotContent, type Width } from "@/shared/panes";

const STEP = 24;

/**
 * A side slot beside the main column whose width the viewer drags (or sets
 * with the arrow keys); each content kind remembers its own width per
 * screen class, so a file browser can stay wide while a thread stays a column.
 * The slot may shrink to MIN_SLOT when the window narrows; the main column
 * gives up room first (it keeps MIN_MAIN, see Shell).
 */
export function SideSlot({ side, content, width, className, children }: { side: "left" | "right"; content: SlotContent; width: Width; className?: string; children: ReactNode }) {
  const { t } = useTranslation("layout");
  const [sizes, setSizes] = useLocalStorage<Record<string, number>>("colony.pane.widths", {});
  const key = `${width}.${content}`;
  const fallback = defaultSlotWidth(content, width);
  const px = sizes[key] ?? fallback;
  const slot = useRef<HTMLDivElement>(null);

  // The room the slot shares with the main column right now.
  const free = () => {
    const el = slot.current;
    const main = el?.parentElement?.querySelector<HTMLElement>("main");
    return (el?.offsetWidth ?? px) + (main?.offsetWidth ?? 0);
  };
  const set = (next: number | null) => {
    const copy = { ...sizes };
    if (next === null) delete copy[key];
    else copy[key] = next;
    setSizes(copy);
  };
  // Dragging the inner edge: towards the main column widens the slot.
  const dir = side === "right" ? -1 : 1;

  const onPointerDown = (e: RPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    e.preventDefault();
    const target = e.currentTarget;
    const startX = e.clientX;
    const start = slot.current?.offsetWidth ?? px;
    const room = free();
    let last = start;
    target.setPointerCapture(e.pointerId);
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    const move = (ev: PointerEvent) => {
      last = clampSlot(start + dir * (ev.clientX - startX), room);
      if (slot.current) slot.current.style.width = `${last}px`;
    };
    const up = () => {
      target.removeEventListener("pointermove", move);
      target.removeEventListener("pointerup", up);
      target.removeEventListener("pointercancel", up);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      if (last !== start) set(last);
    };
    target.addEventListener("pointermove", move);
    target.addEventListener("pointerup", up);
    target.addEventListener("pointercancel", up);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const grow = side === "right" ? "ArrowLeft" : "ArrowRight";
    const shrink = side === "right" ? "ArrowRight" : "ArrowLeft";
    if (e.key !== grow && e.key !== shrink && e.key !== "Home" && e.key !== "End") return;
    e.preventDefault();
    const current = slot.current?.offsetWidth ?? px;
    const next = e.key === "Home" ? MIN_SLOT : e.key === "End" ? Number.MAX_SAFE_INTEGER : current + (e.key === grow ? STEP : -STEP);
    set(clampSlot(next, free()));
  };

  return (
    <div ref={slot} style={{ width: px, minWidth: MIN_SLOT }} className={cn("relative flex shrink overflow-hidden", className)}>
      {children}
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label={t("panes.resize")}
        aria-valuenow={px}
        aria-valuemin={MIN_SLOT}
        tabIndex={0}
        title={t("panes.resizeHint")}
        onPointerDown={onPointerDown}
        onKeyDown={onKeyDown}
        onDoubleClick={() => set(null)}
        className={cn(
          "absolute inset-y-0 z-(--z-pane) w-1.5 cursor-col-resize touch-none transition-colors hover:bg-accent/40 focus-visible:bg-accent/60 focus-visible:outline-none",
          side === "right" ? "left-0" : "right-0",
        )}
      />
    </div>
  );
}
