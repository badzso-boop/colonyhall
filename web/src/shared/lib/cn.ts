// cn joins class names, skipping false, null and undefined:
// cn("row", active && "bg-accent-soft").
export function cn(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(" ");
}
