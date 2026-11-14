// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { ComponentProps, SelectHTMLAttributes } from "react";
import { Checkbox as RCheckbox, Switch as RSwitch } from "radix-ui";
import { Check } from "lucide-react";
import { cn } from "@/shared/lib";

const control =
  "w-full rounded-md border border-border-strong bg-surface text-fg placeholder:text-fg-subtle " +
  "focus-visible:border-fg-muted disabled:opacity-50 aria-invalid:border-danger";

export function Input({ className, ...rest }: ComponentProps<"input">) {
  return <input {...rest} className={cn(control, "h-8 px-2.5", className)} />;
}

// Takes a ref (React 19: ref is a prop), for inserting at the caret.
export function Textarea({ className, ...rest }: ComponentProps<"textarea">) {
  return <textarea {...rest} className={cn(control, "min-h-16 px-2.5 py-2", className)} />;
}

export function Select({ className, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select {...rest} className={cn(control, "h-8 px-2", className)}>
      {children}
    </select>
  );
}

export function Checkbox({ className, ...rest }: RCheckbox.CheckboxProps) {
  return (
    <RCheckbox.Root
      {...rest}
      className={cn(
        "grid size-4 shrink-0 place-items-center rounded-sm border border-border-strong bg-surface",
        "data-[state=checked]:border-accent data-[state=checked]:bg-accent data-[state=checked]:text-accent-fg",
        className,
      )}
    >
      <RCheckbox.Indicator>
        <Check aria-hidden className="size-3" strokeWidth={3} />
      </RCheckbox.Indicator>
    </RCheckbox.Root>
  );
}

export function Switch({ className, ...rest }: RSwitch.SwitchProps) {
  return (
    <RSwitch.Root
      {...rest}
      className={cn(
        "relative inline-flex h-5 w-9 shrink-0 items-center rounded-full border border-border-strong bg-surface-sunken transition-colors",
        "data-[state=checked]:border-accent data-[state=checked]:bg-accent",
        className,
      )}
    >
      <RSwitch.Thumb className="block size-3.5 translate-x-0.5 rounded-full bg-fg-muted transition-transform duration-(--duration-fast) data-[state=checked]:translate-x-[18px] data-[state=checked]:bg-accent-fg" />
    </RSwitch.Root>
  );
}
