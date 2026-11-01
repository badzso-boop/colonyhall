import type { Schemas } from "@/shared/api";
import { compareVersions, enrollCommand, grantsByPath, hostWarnings, isOnline, mmss, newlyEnrolled, secondsLeft } from "./model";

const now = new Date("2026-11-01T10:00:00Z");
const ago = (s: number) => new Date(now.getTime() - s * 1000).toISOString();

function host(extra: Partial<Schemas["Host"]> = {}, report: Partial<Schemas["HostReport"]> = {}): Schemas["Host"] {
  return {
    id: "h1",
    name: "pc",
    color: "blue",
    state: "approved",
    created_at: ago(9999),
    report: { roots: [], clients: [], rejected: [], ...report },
    agents: [],
    ...extra,
  };
}

test("online under 90 seconds, never without last_seen_at", () => {
  expect(isOnline(host({ last_seen_at: ago(89) }), now)).toBe(true);
  expect(isOnline(host({ last_seen_at: ago(90) }), now)).toBe(false);
  expect(isOnline(host(), now)).toBe(false);
});

test("warnings: revoked, rejected paths, unwritable exchange folder, old guard only with a server version", () => {
  expect(hostWarnings(host())).toEqual([]);
  const h = host({ state: "revoked" }, { rejected: [{ agent_id: "a", path: "/x", reason: "outside roots" }], exchange_dir: "/z", exchange_ok: false, guard_version: "1.2.0" });
  expect(hostWarnings(h).map((w) => w.kind)).toEqual(["revoked", "rejected", "exchange"]);
  expect(hostWarnings(h, "1.10.0").map((w) => w.kind)).toContain("oldGuard");
  expect(hostWarnings(host({}, { exchange_dir: "/z", exchange_ok: true }))).toEqual([]);
});

test("versions compare numerically", () => {
  expect(compareVersions("v1.10.0", "1.9.9")).toBe(1);
  expect(compareVersions("1.2", "1.2.0")).toBe(0);
  expect(compareVersions("0.9.1", "0.10")).toBe(-1);
});

test("grants grouped by folder, sorted", () => {
  const agent = (id: string, handle: string, grants: [string, "read" | "write"][]) =>
    ({ agent_id: id, handle, grants: grants.map(([path, mode], i) => ({ id: `${id}${i}`, path, mode, created_at: "" })) }) as unknown as Schemas["HostAgent"];
  const m = grantsByPath({ agents: [agent("b", "zed", [["/p/a", "write"]]), agent("a", "amy", [["/p/a", "read"], ["/p", "read"]])] });
  expect([...m.keys()]).toEqual(["/p", "/p/a"]);
  expect(m.get("/p/a")!.map((g) => `${g.handle}:${g.mode}`)).toEqual(["amy:read", "zed:write"]);
});

test("the enroll command quotes what the shell would split", () => {
  expect(enrollCommand("http://100.85.111.88:8280", "ABCD-1234", "laptop")).toBe("colony guard enroll -server http://100.85.111.88:8280 -code ABCD-1234 -name laptop");
  expect(enrollCommand("http://x", "C", "Norbi gépe")).toBe("colony guard enroll -server http://x -code C -name 'Norbi gépe'");
});

test("countdown", () => {
  expect(secondsLeft(new Date(now.getTime() + 125_500).toISOString(), now)).toBe(125);
  expect(secondsLeft(ago(5), now)).toBe(0);
  expect(mmss(125)).toBe("2:05");
});

test("the newly enrolled host: not known before, preferring the suggested name", () => {
  const hosts = [host({ id: "old" }), host({ id: "n1", name: "other" }), host({ id: "n2", name: "laptop" })];
  expect(newlyEnrolled(hosts, new Set(["old"]), "laptop")?.id).toBe("n2");
  expect(newlyEnrolled(hosts, new Set(["old"]), "x")?.id).toBe("n1");
  expect(newlyEnrolled(hosts, new Set(["old", "n1", "n2"]), "laptop")).toBeNull();
});
