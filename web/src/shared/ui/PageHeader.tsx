// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { ReactNode } from "react";
import { ExternalLink } from "lucide-react";
import { IconButton } from "./IconButton";

/** The header of a page (round 32): icon, title, actions, and a pop-out button when the page has a floating window. */
export function PageHeader({ icon, title, actions, onPopOut, popOutLabel }: { icon?: ReactNode; title: string; actions?: ReactNode; onPopOut?: () => void; popOutLabel?: string }) {
  return (
    <header className="flex min-h-12 shrink-0 flex-wrap items-center gap-2 border-b border-border bg-surface px-4 py-2">
      {icon ? <span className="text-fg-subtle">{icon}</span> : null}
      <h1 className="mr-auto text-page font-semibold">{title}</h1>
      {actions}
      {onPopOut && popOutLabel ? <IconButton label={popOutLabel} icon={<ExternalLink className="size-4" />} onClick={onPopOut} /> : null}
    </header>
  );
}
