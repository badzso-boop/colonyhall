// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo } from "react";
import { cn, foldDiff, lineDiff } from "@/shared/lib";

/** A line diff of two texts, unchanged runs folded (the wiki history, the agent rules). */
export function DiffView({ a, b, skipLabel, label, className }: { a: string; b: string; skipLabel: (count: number) => string; label?: string; className?: string }) {
  const items = useMemo(() => foldDiff(lineDiff(a, b)), [a, b]);
  return (
    <pre aria-label={label} className={cn("overflow-auto rounded-md border border-border bg-surface-sunken p-2 font-mono text-meta leading-relaxed", className)}>
      {items.map((l, i) =>
        l.op === "skip" ? (
          <div key={i} className="px-1 py-0.5 text-center text-fg-subtle italic">
            {skipLabel(l.count)}
          </div>
        ) : (
          <div key={i} className={cn("px-1 whitespace-pre-wrap", l.op === "add" && "bg-success-soft text-success", l.op === "del" && "bg-danger-soft text-danger line-through")}>
            {l.op === "add" ? "+ " : l.op === "del" ? "− " : "  "}
            {l.text || " "}
          </div>
        ),
      )}
    </pre>
  );
}
