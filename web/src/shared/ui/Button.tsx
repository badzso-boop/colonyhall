// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Loader2 } from "lucide-react";
import { cn } from "@/shared/lib";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "sm" | "md";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-accent text-accent-fg hover:bg-accent-hover",
  secondary: "border border-border-strong bg-surface text-fg hover:bg-surface-sunken",
  ghost: "text-fg-muted hover:bg-surface-sunken hover:text-fg",
  danger: "border border-danger text-danger hover:bg-danger-soft",
};

const sizes: Record<ButtonSize, string> = {
  sm: "h-7 gap-1.5 px-2.5 text-ui",
  md: "h-8 gap-2 px-3 text-body",
};

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  icon?: ReactNode;
};

export function Button({ variant = "secondary", size = "md", loading, icon, className, children, disabled, ...rest }: ButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-md font-medium whitespace-nowrap transition-colors duration-(--duration-fast)",
        "disabled:cursor-not-allowed disabled:opacity-50",
        variants[variant],
        sizes[size],
        className,
      )}
    >
      {loading ? <Loader2 aria-hidden className="size-4 animate-spin motion-reduce:animate-none" /> : icon}
      {children}
    </button>
  );
}
