// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { X } from "lucide-react";
import { cn } from "@/shared/lib";
import { IconButton } from "./IconButton";
import { useToasts } from "./toast";

/** Renders the toasts: bottom right on desktop, top on a phone. Mounted once in app/. */
export function Toaster({ closeLabel }: { closeLabel: string }) {
  const items = useToasts((s) => s.items);
  const dismiss = useToasts((s) => s.dismiss);
  return (
    <div
      aria-live="polite"
      className="pointer-events-none fixed inset-x-3 top-3 z-(--z-toast) flex flex-col gap-2 sm:inset-x-auto sm:top-auto sm:right-4 sm:bottom-4 sm:w-80"
    >
      {items.map((t) => (
        <div
          key={t.id}
          role="status"
          className={cn(
            "pointer-events-auto flex items-center gap-2 rounded-lg border bg-surface-raised py-2 pr-1 pl-3 text-ui shadow-overlay",
            t.tone === "danger" ? "border-danger" : "border-border",
          )}
        >
          <span className="min-w-0 flex-1">{t.text}</span>
          {t.action ? (
            <button
              type="button"
              className="rounded-md px-2 py-1 font-semibold text-accent-text hover:bg-surface-sunken"
              onClick={() => {
                t.action?.run();
                dismiss(t.id);
              }}
            >
              {t.action.label}
            </button>
          ) : null}
          <IconButton label={closeLabel} icon={<X className="size-4" />} size="sm" onClick={() => dismiss(t.id)} />
        </div>
      ))}
    </div>
  );
}
