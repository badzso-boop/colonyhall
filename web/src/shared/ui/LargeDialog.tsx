// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type ReactNode } from "react";
import { Dialog as RDialog } from "radix-ui";
import { ChevronLeft, ChevronRight, X } from "lucide-react";
import { useWidth } from "@/shared/hooks";
import { cn } from "@/shared/lib";
import { IconButton } from "./IconButton";

export type LargeDialogTab = {
  id: string;
  label: string;
  /** A step number or a small mark (🔒) before the label. */
  marker?: ReactNode;
  /** A step that has an error: shown in danger. */
  invalid?: boolean;
  disabled?: boolean;
};

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  /** Under the title: an avatar, a host chip… */
  header?: ReactNode;
  tabs: LargeDialogTab[];
  active: string;
  onSelect: (id: string) => void;
  /** "Lépések" or "Fülek": the navigation's accessible name. */
  navLabel: string;
  /** Above every tab's content (e.g. the immutable-configuration warning). */
  banner?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  closeLabel: string;
  backLabel: string;
  /** Asked before closing (unsaved input); return false to keep it open. */
  beforeClose?: () => boolean;
};

/**
 * The large Dialog (docs/07, round 33): max-w-4xl, at most 85 vh, tabs or
 * steps on the left, the content scrolling on the right, a sticky footer.
 * On a phone a full-screen sheet whose first screen is the tab list.
 * One child Dialog (a confirmation, the folder picker) may open on top.
 */
export function LargeDialog({ open, onOpenChange, title, header, tabs, active, onSelect, navLabel, banner, children, footer, closeLabel, backLabel, beforeClose }: Props) {
  const narrow = useWidth() === "narrow";
  const [phoneNav, setPhoneNav] = useState(true);
  const close = (o: boolean) => {
    if (!o && beforeClose && !beforeClose()) return;
    onOpenChange(o);
  };
  const select = (id: string) => {
    onSelect(id);
    setPhoneNav(false);
  };
  const current = tabs.find((t) => t.id === active);
  const nav = (
    <nav aria-label={navLabel} className={cn("flex shrink-0 flex-col gap-px overflow-y-auto p-2", narrow ? "w-full" : "w-56 border-r border-border")}>
      {tabs.map((t) => (
        <button
          key={t.id}
          type="button"
          disabled={t.disabled}
          aria-current={t.id === active ? "step" : undefined}
          onClick={() => select(t.id)}
          className={cn(
            "flex h-9 items-center gap-2 rounded-md px-2.5 text-left text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg disabled:opacity-50",
            t.id === active && !narrow && "bg-accent-soft font-semibold text-fg",
            t.invalid && "text-danger",
          )}
        >
          {t.marker ? <span className="grid min-w-5 place-items-center text-meta tabular-nums">{t.marker}</span> : null}
          <span className="flex-1 truncate">{t.label}</span>
          {narrow ? <ChevronRight aria-hidden className="size-4 text-fg-subtle" /> : null}
        </button>
      ))}
    </nav>
  );
  const showNav = narrow && phoneNav;
  return (
    <RDialog.Root open={open} onOpenChange={close}>
      <RDialog.Portal>
        <RDialog.Overlay className="fixed inset-0 z-(--z-overlay) bg-overlay backdrop-blur-[2px]" />
        <RDialog.Content
          aria-describedby={undefined}
          className={cn(
            "fixed z-(--z-overlay) flex flex-col border border-border bg-surface-raised shadow-overlay",
            narrow ? "inset-0" : "top-1/2 left-1/2 h-[85dvh] w-[calc(100vw-2rem)] max-w-4xl -translate-x-1/2 -translate-y-1/2 rounded-lg",
          )}
        >
          <div className="flex items-start gap-3 border-b border-border px-4 py-3">
            {narrow && !showNav ? <IconButton size="sm" label={backLabel} icon={<ChevronLeft className="size-4" />} onClick={() => setPhoneNav(true)} /> : null}
            <div className="min-w-0 flex-1">
              <RDialog.Title className="truncate text-title font-semibold">{narrow && !showNav && current ? current.label : title}</RDialog.Title>
              {header && (!narrow || showNav) ? <div className="mt-1">{header}</div> : null}
            </div>
            <RDialog.Close asChild>
              <IconButton label={closeLabel} icon={<X className="size-4" />} size="sm" />
            </RDialog.Close>
          </div>
          <div className="flex min-h-0 flex-1">
            {!narrow || showNav ? nav : null}
            {!showNav ? (
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="min-h-0 flex-1 overflow-y-auto p-4">
                  {banner ? <div className="mb-4">{banner}</div> : null}
                  {children}
                </div>
                {footer ? <div className="flex items-center justify-end gap-2 border-t border-border px-4 py-3">{footer}</div> : null}
              </div>
            ) : null}
          </div>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}
