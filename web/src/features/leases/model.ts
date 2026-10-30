import type { Schemas } from "@/shared/api";

export type Lease = Schemas["Lease"];
export type LeaseState = "active" | "expired" | "released";
export type SortKey = "resource" | "holder" | "note" | "acquired" | "expires" | "state";

export function leaseState(l: Lease, now: Date): LeaseState {
  if (l.released_at) return "released";
  return new Date(l.expires_at).getTime() <= now.getTime() ? "expired" : "active";
}

/** "4:59", "1:02:03"; null once it is over. */
export function countdown(l: Lease, now: Date): string | null {
  const ms = new Date(l.expires_at).getTime() - now.getTime();
  if (ms <= 0) return null;
  const s = Math.ceil(ms / 1000);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = String(s % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

/** The share of the lease's time still left, 0…1 (0 when released). */
export function remaining(l: Lease, now: Date): number {
  if (l.released_at) return 0;
  const start = new Date(l.acquired_at).getTime();
  const end = new Date(l.expires_at).getTime();
  if (end <= start) return 0;
  return Math.min(1, Math.max(0, (end - now.getTime()) / (end - start)));
}

export const Soon = 5 * 60_000;

const order: Record<LeaseState, number> = { active: 0, expired: 1, released: 2 };

export function sortLeases(list: Lease[], key: SortKey, desc: boolean, now: Date): Lease[] {
  const val = (l: Lease): string | number => {
    switch (key) {
      case "resource":
        return l.resource;
      case "holder":
        return l.holder_handle;
      case "note":
        return l.note ?? "";
      case "acquired":
        return l.acquired_at;
      case "expires":
        return l.expires_at;
      case "state":
        return order[leaseState(l, now)];
    }
  };
  return [...list].sort((a, b) => {
    const x = val(a);
    const y = val(b);
    const c = typeof x === "number" ? x - (y as number) : x.localeCompare(y as string);
    return (desc ? -c : c) || a.id.localeCompare(b.id);
  });
}

/** Free text over resource and note, and an optional holder. */
export function filterLeases(list: Lease[], text: string, holder: string): Lease[] {
  const q = text.trim().toLowerCase();
  return list.filter((l) => (!holder || l.holder_id === holder) && (!q || l.resource.toLowerCase().includes(q) || (l.note ?? "").toLowerCase().includes(q)));
}
