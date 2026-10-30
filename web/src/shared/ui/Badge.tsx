import type { ReactNode } from "react";
import { cn } from "@/shared/lib";

export type BadgeTone = "neutral" | "accent" | "success" | "warning" | "info" | "danger";

const tones: Record<BadgeTone, string> = {
  neutral: "bg-surface-sunken text-fg-muted",
  accent: "bg-accent text-accent-fg",
  success: "bg-success-soft text-success",
  warning: "bg-warning-soft text-warning",
  info: "bg-info-soft text-info",
  danger: "bg-danger-soft text-danger",
};

/** A small label: a count, a status chip, or (mono) a git letter. */
export function Badge({ tone = "neutral", mono, children, className }: { tone?: BadgeTone; mono?: boolean; children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "tabular inline-flex h-5 min-w-5 items-center justify-center gap-1 rounded-full px-1.5 text-meta font-semibold",
        mono && "rounded-sm font-mono",
        tones[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
