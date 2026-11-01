import type { ReactNode } from "react";
import { Dialog as RDialog, DropdownMenu as RMenu, Popover as RPopover, Tabs as RTabs, Tooltip as RTooltip } from "radix-ui";
import { X } from "lucide-react";
import { cn } from "@/shared/lib";
import { IconButton } from "./IconButton";

/* ---------- Tooltip ---------- */

export const TooltipProvider = RTooltip.Provider;

export function Tooltip({ content, children }: { content: ReactNode; children: ReactNode }) {
  return (
    <RTooltip.Root delayDuration={300}>
      <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
      <RTooltip.Portal>
        <RTooltip.Content
          sideOffset={6}
          className="z-(--z-toast) max-w-xs rounded-md bg-fg px-2 py-1 text-meta text-bg shadow-overlay"
        >
          {content}
        </RTooltip.Content>
      </RTooltip.Portal>
    </RTooltip.Root>
  );
}

/* ---------- Dialog and Sheet (docs/07: the overlay rules) ---------- */

const overlay = "fixed inset-0 z-(--z-overlay) bg-overlay backdrop-blur-[2px]";

type DialogProps = {
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  /** md (default) and lg are short forms and confirmations; xl is the large settings dialog. */
  size?: "md" | "lg" | "xl";
  closeLabel: string;
};

/**
 * A centred dialog; below 640 px it becomes a bottom sheet. Esc and the
 * overlay close it; focus is trapped and returns to the trigger (Radix).
 */
export function Dialog({ open, onOpenChange, title, description, children, footer, size = "md", closeLabel }: DialogProps) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className={overlay} />
        <RDialog.Content
          className={cn(
            "fixed z-(--z-overlay) flex flex-col border border-border bg-surface-raised shadow-overlay",
            "inset-x-0 bottom-0 max-h-[92dvh] rounded-t-lg",
            "sm:inset-auto sm:top-1/2 sm:left-1/2 sm:max-h-[85dvh] sm:w-[calc(100vw-2rem)] sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-lg",
            size === "md" && "sm:max-w-md",
            size === "lg" && "sm:max-w-lg",
            size === "xl" && "sm:max-w-4xl",
          )}
        >
          <div aria-hidden className="mx-auto mt-2 h-1 w-10 rounded-full bg-border-strong sm:hidden" />
          <div className="flex items-start gap-3 px-4 pt-3 pb-2">
            <div className="min-w-0 flex-1">
              <RDialog.Title className="text-title font-semibold">{title}</RDialog.Title>
              {description ? <RDialog.Description className="mt-0.5 text-ui text-fg-muted">{description}</RDialog.Description> : null}
            </div>
            <RDialog.Close asChild>
              <IconButton label={closeLabel} icon={<X className="size-4" />} size="sm" />
            </RDialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">{children}</div>
          {footer ? <div className="flex justify-end gap-2 border-t border-border px-4 py-3">{footer}</div> : null}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

/** Longer read-only detail: from the right on desktop, from the bottom on a phone. */
export function Sheet({ open, onOpenChange, title, description, children, closeLabel }: Omit<DialogProps, "size" | "footer">) {
  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className={overlay} />
        <RDialog.Content
          className={cn(
            "fixed z-(--z-overlay) flex flex-col border-border bg-surface-raised shadow-overlay",
            "inset-x-0 bottom-0 max-h-[92dvh] rounded-t-lg border-t",
            "sm:inset-y-0 sm:right-0 sm:left-auto sm:max-h-none sm:w-[min(32rem,100vw)] sm:rounded-none sm:border-t-0 sm:border-l",
          )}
        >
          <div className="flex items-start gap-3 border-b border-border px-4 py-3">
            <div className="min-w-0 flex-1">
              <RDialog.Title className="text-title font-semibold">{title}</RDialog.Title>
              {description ? <RDialog.Description className="text-ui text-fg-muted">{description}</RDialog.Description> : null}
            </div>
            <RDialog.Close asChild>
              <IconButton label={closeLabel} icon={<X className="size-4" />} size="sm" />
            </RDialog.Close>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-4">{children}</div>
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

/* ---------- Dropdown menu ---------- */

export const Menu = RMenu.Root;
export const MenuTrigger = RMenu.Trigger;

export function MenuContent({ children, align = "end" }: { children: ReactNode; align?: "start" | "end" }) {
  return (
    <RMenu.Portal>
      <RMenu.Content
        align={align}
        sideOffset={4}
        className="z-(--z-overlay) min-w-44 rounded-md border border-border bg-surface-raised p-1 text-ui shadow-overlay"
      >
        {children}
      </RMenu.Content>
    </RMenu.Portal>
  );
}

export function MenuItem({ children, onSelect, danger, icon }: { children: ReactNode; onSelect?: () => void; danger?: boolean; icon?: ReactNode }) {
  return (
    <RMenu.Item
      onSelect={onSelect}
      className={cn(
        "flex h-8 cursor-pointer items-center gap-2 rounded-sm px-2 outline-none select-none data-highlighted:bg-surface-sunken",
        danger ? "text-danger" : "text-fg",
      )}
    >
      {icon}
      {children}
    </RMenu.Item>
  );
}

export function MenuSeparator() {
  return <RMenu.Separator className="my-1 h-px bg-border" />;
}

/* ---------- Popover ---------- */

export const Popover = RPopover.Root;
export const PopoverTrigger = RPopover.Trigger;
export const PopoverAnchor = RPopover.Anchor;

/** A small anchored panel (event detail, a calendar list); Esc and an outside click close it. */
export function PopoverContent({ children, label, align = "start", className }: { children: ReactNode; label: string; align?: "start" | "center" | "end"; className?: string }) {
  return (
    <RPopover.Portal>
      <RPopover.Content
        aria-label={label}
        align={align}
        sideOffset={6}
        collisionPadding={12}
        className={cn("z-(--z-overlay) w-80 max-w-[calc(100vw-2rem)] rounded-lg border border-border bg-surface-raised p-3 text-ui shadow-overlay", className)}
      >
        {children}
      </RPopover.Content>
    </RPopover.Portal>
  );
}

/* ---------- Tabs ---------- */

export const Tabs = RTabs.Root;
export const TabPanel = RTabs.Content;

export function TabList({ children, label, vertical }: { children: ReactNode; label: string; vertical?: boolean }) {
  return (
    <RTabs.List aria-label={label} className={cn("flex gap-1", vertical ? "flex-col" : "border-b border-border")}>
      {children}
    </RTabs.List>
  );
}

export function Tab({ value, children }: { value: string; children: ReactNode }) {
  return (
    <RTabs.Trigger
      value={value}
      className={cn(
        "h-8 rounded-md px-3 text-ui text-fg-muted hover:text-fg",
        "data-[state=active]:bg-accent-soft data-[state=active]:font-semibold data-[state=active]:text-fg",
      )}
    >
      {children}
    </RTabs.Trigger>
  );
}
