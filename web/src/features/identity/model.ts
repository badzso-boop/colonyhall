/**
 * Where to go after logging in: only a path inside the app, so a crafted
 * link cannot send the user to another site (?next=https://evil).
 */
export function safeNext(next: string | null): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/login")) return "/";
  return next;
}
