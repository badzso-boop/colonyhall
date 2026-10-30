import { useEffect, useRef, useState, type ReactNode } from "react";
import { cn } from "@/shared/lib";

/**
 * Press and hold to confirm (the emergency stop, docs/01-terv.md): no
 * dialog, fast on a phone, but not by accident. Works with the pointer and
 * with Space/Enter held down. Releasing early cancels.
 */
export function HoldButton({
  label,
  hint,
  icon,
  holdMs = 2000,
  onConfirm,
  className,
  compact,
}: {
  label: string;
  /** Tooltip and accessible description, e.g. "Nyomd és tartsd 2 másodpercig". */
  hint: string;
  icon?: ReactNode;
  holdMs?: number;
  onConfirm: () => void;
  className?: string;
  /** Icon only (the label stays the accessible name). */
  compact?: boolean;
}) {
  const [progress, setProgress] = useState(0);
  const timer = useRef<number | null>(null);
  const started = useRef(0);

  const stop = () => {
    if (timer.current !== null) window.clearInterval(timer.current);
    timer.current = null;
    setProgress(0);
  };
  const start = () => {
    if (timer.current !== null) return;
    started.current = Date.now();
    timer.current = window.setInterval(() => {
      const p = Math.min(1, (Date.now() - started.current) / holdMs);
      setProgress(p);
      if (p >= 1) {
        stop();
        onConfirm();
      }
    }, 50);
  };
  useEffect(() => stop, []);

  return (
    <button
      type="button"
      aria-label={label}
      aria-description={hint}
      title={hint}
      onPointerDown={start}
      onPointerUp={stop}
      onPointerLeave={stop}
      onPointerCancel={stop}
      onKeyDown={(e) => {
        if ((e.key === " " || e.key === "Enter") && !e.repeat) {
          e.preventDefault();
          start();
        }
      }}
      onKeyUp={(e) => {
        if (e.key === " " || e.key === "Enter") stop();
      }}
      onContextMenu={(e) => e.preventDefault()}
      className={cn(
        "relative inline-flex h-8 items-center gap-2 overflow-hidden rounded-md border-[1.5px] border-accent px-3 text-ui font-semibold text-accent-text select-none",
        "hover:bg-accent-soft",
        className,
      )}
    >
      <span
        aria-hidden
        className="absolute inset-y-0 left-0 bg-accent/25"
        style={{ width: `${Math.round(progress * 100)}%` }}
      />
      <span className="relative inline-flex items-center gap-2">
        {icon}
        {compact ? null : label}
      </span>
    </button>
  );
}
