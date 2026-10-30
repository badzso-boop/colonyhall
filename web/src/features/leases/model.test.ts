import { countdown, filterLeases, leaseState, remaining, sortLeases, type Lease } from "./model";

const now = new Date("2026-10-30T10:00:00Z");
const lease = (id: string, resource: string, acquired: string, expires: string, extra: Partial<Lease> = {}): Lease => ({
  id, workspace_id: "w", resource, holder_id: `h-${id}`, holder_handle: `agent-${id}`, note: `note ${id}`, acquired_at: acquired, expires_at: expires, ...extra,
});

test("state, countdown and the remaining share", () => {
  const a = lease("a", "db", "2026-10-30T09:50:00Z", "2026-10-30T10:04:30Z");
  expect([leaseState(a, now), countdown(a, now)]).toEqual(["active", "4:30"]);
  expect(remaining(a, now)).toBeCloseTo(270 / 870);
  const long = lease("l", "x", "2026-10-30T09:00:00Z", "2026-10-30T11:02:03Z");
  expect(countdown(long, now)).toBe("1:02:03");
  const gone = lease("g", "x", "2026-10-30T08:00:00Z", "2026-10-30T09:00:00Z");
  expect([leaseState(gone, now), countdown(gone, now), remaining(gone, now)]).toEqual(["expired", null, 0]);
  const rel = lease("r", "x", "2026-10-30T09:00:00Z", "2026-10-30T11:00:00Z", { released_at: "2026-10-30T09:30:00Z" });
  expect([leaseState(rel, now), remaining(rel, now)]).toEqual(["released", 0]);
});

test("sorting by any column and filtering", () => {
  const list = [
    lease("b", "repo/main", "2026-10-30T09:00:00Z", "2026-10-30T12:00:00Z"),
    lease("a", "db/migrate", "2026-10-30T09:30:00Z", "2026-10-30T09:40:00Z"),
    lease("c", "deploy", "2026-10-30T08:00:00Z", "2026-10-30T13:00:00Z", { released_at: "2026-10-30T08:10:00Z", note: "kész" }),
  ];
  expect(sortLeases(list, "resource", false, now).map((l) => l.id)).toEqual(["a", "c", "b"]);
  expect(sortLeases(list, "acquired", true, now).map((l) => l.id)).toEqual(["a", "b", "c"]);
  expect(sortLeases(list, "state", false, now).map((l) => l.id)).toEqual(["b", "a", "c"]);
  expect(filterLeases(list, "KÉSZ", "").map((l) => l.id)).toEqual(["c"]);
  expect(filterLeases(list, "", "h-a").map((l) => l.id)).toEqual(["a"]);
});
