import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "@/shared/lib";

export type IconButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label" | "children"> & {
  /** Required: an icon alone has no accessible name. */
  label: string;
  icon: ReactNode;
  size?: "sm" | "md";
  /** Shows a count badge in the corner. */
  badge?: number;
};

export function IconButton({ label, icon, size = "md", badge, className, ...rest }: IconButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      aria-label={badge ? `${label} (${badge})` : label}
      title={label}
      className={cn(
        "relative inline-grid shrink-0 place-items-center rounded-md text-fg-muted transition-colors duration-(--duration-fast)",
        "hover:bg-surface-sunken hover:text-fg disabled:opacity-50",
        size === "sm" ? "size-7" : "size-8",
        className,
      )}
    >
      {icon}
      {badge ? (
        <span className="tabular absolute -top-0.5 -right-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-accent px-1 text-[10px] leading-none font-semibold text-accent-fg">
          {badge > 99 ? "99+" : badge}
        </span>
      ) : null}
    </button>
  );
}
