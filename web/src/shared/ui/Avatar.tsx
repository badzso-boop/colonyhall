// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { CSSProperties } from "react";
import { cn } from "@/shared/lib";

export type AvatarKind = "human" | "agent";
export type AvatarStatus = "working" | "idle" | "offline" | "paused";
export type AvatarSize = "sm" | "md" | "lg";

const sizes: Record<AvatarSize, string> = {
  sm: "size-5 text-[11px]",
  md: "size-7 text-ui",
  lg: "size-9 text-title",
};

/**
 * Humans are circles, agents rounded squares: the main way to tell them
 * apart at a glance (docs/07). Agents get a ring in their colour (the host's
 * by default, round 33); colour is never the only signal.
 */
export function Avatar({
  kind,
  name,
  emoji,
  status,
  size = "md",
  ringColor,
  className,
}: {
  kind: AvatarKind;
  name: string;
  emoji?: string;
  status?: AvatarStatus;
  size?: AvatarSize;
  /** A CSS colour value from the palette tokens, e.g. "var(--color-info)". */
  ringColor?: string;
  className?: string;
}) {
  const initial = name.trim().charAt(0).toUpperCase() || "?";
  const style: CSSProperties | undefined = kind === "agent" ? { boxShadow: `0 0 0 2px ${ringColor ?? "var(--color-border-strong)"}` } : undefined;
  return (
    <span
      role="img"
      aria-label={name}
      data-kind={kind}
      style={style}
      className={cn(
        "relative inline-grid shrink-0 place-items-center font-semibold select-none",
        kind === "human" ? "rounded-full border border-border bg-surface-sunken text-fg" : "rounded-md bg-surface-raised",
        sizes[size],
        className,
      )}
    >
      <span aria-hidden>{emoji || initial}</span>
      {status ? <StatusDot status={status} /> : null}
    </span>
  );
}

function StatusDot({ status }: { status: AvatarStatus }) {
  return (
    <span
      aria-hidden
      data-status={status}
      className={cn(
        "absolute -right-1 -bottom-1 size-2.5 rounded-full border-2 border-surface",
        status === "working" && "bg-accent motion-safe:animate-pulse",
        status === "idle" && "bg-fg-subtle",
        status === "offline" && "bg-surface shadow-[inset_0_0_0_2px_var(--color-fg-subtle)]",
        status === "paused" && "bg-warning",
      )}
    />
  );
}
