import { describe, expect, test } from "vitest";
import {
  agentRows,
  assignHues,
  auditExportQuery,
  dayList,
  generatePassword,
  OTHER_KEY,
  payloadRows,
  periodDays,
  periodTiles,
  priceHistory,
  sortRows,
  stackDays,
  searchTone,
  statusFindings,
  formatBytes,
  sizeGrowth,
  statusText,
  toHuf,
  validRepo,
  wizardReducer,
  wizardStart,
  type AdminStatus,
  type UsageGroup,
} from "./model";

const group = (key: string, day: string, total: number, extra: Partial<UsageGroup> = {}): UsageGroup => ({
  key,
  day,
  input_tokens: total,
  output_tokens: 0,
  cache_read_tokens: 0,
  cache_write_tokens: 0,
  total_tokens: total,
  run_count: 1,
  error_count: 0,
  ...extra,
});

describe("period", () => {
  test("a fixed period ends today", () => {
    expect(periodDays({ kind: "7" }, "2026-11-02")).toEqual({ from: "2026-10-27", to: "2026-11-02", fetchDays: 7 });
    expect(periodDays({ kind: "today" }, "2026-11-02").fetchDays).toBe(1);
  });
  test("a custom period is cut to 90 days back and to today", () => {
    const p = periodDays({ kind: "custom", from: "2020-01-01", to: "2030-01-01" }, "2026-11-02");
    expect(p.to).toBe("2026-11-02");
    expect(p.fetchDays).toBe(90);
    expect(dayList(p.from, p.to)).toHaveLength(90);
  });
  test("a reversed custom period does not break", () => {
    const p = periodDays({ kind: "custom", from: "2026-11-01", to: "2026-10-01" }, "2026-11-02");
    expect(p.from).toBe(p.to);
  });
});

test("HUF comes from the rate, and no rate means no amount", () => {
  expect(toHuf(2, 400)).toBe(800);
  expect(toHuf(2, undefined)).toBeNull();
  expect(toHuf(2, 0)).toBeNull();
});

describe("hues", () => {
  test("own colours win, and removing a series does not repaint the others", () => {
    const all = assignHues(["a", "b", "c"], (k) => (k === "a" ? "red" : undefined));
    expect(all.get("a")).toBe("red");
    const fewer = assignHues(["b"], () => undefined);
    const b = fewer.get("b");
    expect(typeof b).toBe("string");
    expect(assignHues(["b"], () => undefined).get("b")).toBe(b);
  });
  test("colliding names still get different hues", () => {
    const hues = assignHues(["a", "b", "c", "d", "e", "f"], () => undefined);
    expect(new Set(hues.values()).size).toBe(6);
  });
});

describe("stackDays", () => {
  test("folds the small series into other, and fills the empty days", () => {
    const groups = [group("a", "2026-11-01", 100), group("b", "2026-11-01", 50), group("c", "2026-11-02", 1), group("old", "2026-01-01", 999)];
    const out = stackDays(groups, ["2026-11-01", "2026-11-02", "2026-11-03"], (g) => g.total_tokens, 2);
    expect(out.keys).toEqual(["a", "b", OTHER_KEY]);
    expect(out.days[1]?.segments).toEqual([{ key: OTHER_KEY, value: 1 }]);
    expect(out.days[2]?.total).toBe(0);
  });
});

test("the tiles count only the period", () => {
  const groups = [group("a", "2026-11-01", 10, { estimated_usd: 1.5, error_count: 1 }), group("a", "2026-10-01", 99, { estimated_usd: 9 })];
  expect(periodTiles(groups, "2026-11-01", "2026-11-02")).toEqual({ tokens: 10, runs: 1, failures: 1, usd: 1.5, hasUsd: true });
});

test("agent rows add up, and no price stays null rather than zero", () => {
  const rows = agentRows([group("a", "2026-11-01", 10), group("a", "2026-11-02", 5)], ["2026-11-01", "2026-11-02"]);
  expect(rows[0]).toMatchObject({ runs: 2, input: 15, usd: null, lastDay: "2026-11-02", spark: [10, 5] });
});

test("sorting keeps empty values last in both directions", () => {
  const rows = [{ v: 2 }, { v: null }, { v: 5 }];
  expect(sortRows(rows, (r) => r.v, "desc").map((r) => r.v)).toEqual([5, 2, null]);
  expect(sortRows(rows, (r) => r.v, "asc").map((r) => r.v)).toEqual([2, 5, null]);
});

const status = (over: Partial<NonNullable<AdminStatus["system"]>> = {}): AdminStatus => ({
  emergency_stop: { active: false },
  system: {
    version: "v1",
    uptime_seconds: 3700,
    database: { ok: true, latency_ms: 1.2 },
    event_head: 10,
    consumers: [{ name: "audit", offset: 10, lag: 0, lag_seconds: 0 }],
    connections: { human: 1, agent: 0, host: 1 },
    guards: [{ id: "g", name: "pc", online: true, guard_version: "v1", last_seen_at: "2026-11-02T10:00:00Z" }],
    scheduler: { failed_24h: 0 },
    compaction: { running: 0, failed_24h: 0 },
    gitea_configured: true,
    gitea_webhook_configured: false,
    max_upload_mb: 25,
    security: { lockouts_24h: 0, lockouts: [], allowlist: [], trusted_proxies: false },
    ...over,
  },
});
const tone = (st: AdminStatus, id: string) => statusFindings(st).find((f) => f.id === id)?.tone;

describe("status thresholds", () => {
  test("a healthy system is all ok", () => {
    expect(statusFindings(status()).every((f) => f.tone === "ok")).toBe(true);
  });
  test("a lockout in the last 24 hours warns (round 40)", () => {
    expect(tone(status({ security: { lockouts_24h: 1, lockouts: [], allowlist: [], trusted_proxies: false } }), "security")).toBe("warning");
  });
  test("consumer lag warns above 100 events or 60 s, not at them", () => {
    const lag = (l: number, s: number) => tone(status({ consumers: [{ name: "a", offset: 0, lag: l, lag_seconds: s }] }), "events");
    expect(lag(100, 60)).toBe("ok");
    expect(lag(101, 0)).toBe("warning");
    expect(lag(0, 61)).toBe("warning");
  });
  test("an offline guard and a failed schedule warn; a dead database and a stop are errors", () => {
    expect(tone(status({ guards: [{ id: "g", name: "pc", online: false }] }), "guards")).toBe("warning");
    expect(tone(status({ scheduler: { failed_24h: 1 } }), "scheduler")).toBe("warning");
    expect(tone(status({ database: { ok: false, latency_ms: 0 } }), "database")).toBe("error");
    expect(tone({ ...status(), emergency_stop: { active: true } }, "emergency")).toBe("error");
  });
  test("search: a mismatch is an error, an unreachable configured backend a warning, an unconfigured one fine", () => {
    const search = { chunks: 1, pending: 0, models: ["m"], embedder: { configured: true, reachable: true }, reranker: { configured: false, reachable: false } };
    expect(tone(status(), "search")).toBeUndefined();
    expect(tone(status({ search }), "search")).toBe("ok");
    expect(searchTone({ ...search, reranker: { configured: true, reachable: false } })).toBe("warning");
    expect(searchTone({ ...search, embedder: { configured: true, reachable: false } })).toBe("warning");
    expect(searchTone({ ...search, model_mismatch: "x" })).toBe("error");
    const text = statusText(status({ search: { ...search, model_mismatch: "stored a, served b" } }), new Date("2026-11-02T10:00:00Z"));
    expect(text).toContain("search: 1 chunks, 0 waiting for embedding");
    expect(text).toContain("reranker: not configured");
    expect(text).toContain("MODEL MISMATCH: stored a, served b");
  });
  test("the copied summary names the facts", () => {
    const text = statusText(status(), new Date("2026-11-02T10:00:00Z"));
    expect(text).toContain("consumer audit: offset 10, lag 0");
    expect(text).toContain("webhook secret: no");
  });
});

test("a generated password has the asked length and only the safe alphabet", () => {
  const p = generatePassword(24);
  expect(p).toHaveLength(24);
  expect(p).toMatch(/^[a-km-np-zA-HJ-NP-Z2-9]+$/);
});

test("a generated password discards biased values", () => {
  const values = [0xffffffff, 0];
  let i = 0;
  const random = (a: Uint32Array) => a.map(() => values[i++ % 2]!);
  expect(generatePassword(4, random)).toBe("aaaa");
});

test("repo names are owner/repo", () => {
  expect(validRepo("colony-agents/visegrad")).toBe(true);
  expect(validRepo("visegrad")).toBe(false);
  expect(validRepo("a/b/c")).toBe(false);
});

test("the price in force comes first per model", () => {
  const out = priceHistory([
    { model: "m", valid_from: "2026-01-01T00:00:00Z" },
    { model: "m", valid_from: "2026-06-01T00:00:00Z" },
    { model: "a*", valid_from: "2026-01-01T00:00:00Z" },
  ]);
  expect(out.map((g) => g.model)).toEqual(["a*", "m"]);
  expect(out[1]?.rows[0]?.valid_from).toBe("2026-06-01T00:00:00Z");
});

describe("owner key wizard", () => {
  test("step 2 stays closed until the new key is saved", () => {
    const created = wizardReducer(wizardStart, { type: "created", tokenId: "t" });
    expect(wizardReducer(created, { type: "goto", step: 2 }).step).toBe(1);
    const saved = wizardReducer(created, { type: "saved", saved: true });
    expect(wizardReducer(saved, { type: "goto", step: 2 }).step).toBe(2);
  });
  test("its state holds no secret and survives JSON", () => {
    const s = wizardReducer(wizardReducer(wizardStart, { type: "created", tokenId: "t" }), { type: "place", place: "env", on: true });
    expect(JSON.parse(JSON.stringify(s))).toEqual({ step: 1, newTokenId: "t", saved: false, places: ["env"], done: false });
  });
  test("finishing resets and remembers that it finished", () => {
    expect(wizardReducer({ ...wizardStart, step: 3, saved: true, newTokenId: "t" }, { type: "finish" })).toEqual({ ...wizardStart, done: true });
  });
});

test("the payload stays text", () => {
  expect(payloadRows({ a: "<b>x</b>", n: 2, o: { k: 1 } })).toEqual([
    { key: "a", value: "<b>x</b>" },
    { key: "n", value: "2" },
    { key: "o", value: '{"k":1}' },
  ]);
  expect(payloadRows(null)).toEqual([]);
});

test("the export query repeats the actions", () => {
  expect(auditExportQuery({ action: ["a.b", "c.d"], target_type: "actor" })).toBe("action=a.b&action=c.d&target_type=actor");
});

test("byte counts read like pg_size_pretty", () => {
  expect(formatBytes(0, "en")).toBe("0 B");
  expect(formatBytes(1023, "en")).toBe("1,023 B");
  expect(formatBytes(1536, "hu")).toBe("1,5 kB");
  expect(formatBytes(21.44 * 1024 * 1024, "en")).toBe("21.4 MB");
  expect(formatBytes(3 * 1024 ** 3, "en")).toBe("3 GB");
  expect(formatBytes(-2048, "en")).toBe("-2 kB");
});

test("the growth compares the newest sample with the oldest of the last 30 days", () => {
  const s = (date: string, size_bytes: number) => ({ date, size_bytes });
  expect(sizeGrowth([])).toBeNull();
  expect(sizeGrowth([s("2026-11-11", 5)])).toBeNull();
  expect(sizeGrowth([s("2026-09-01", 1), s("2026-11-11", 5)])).toBeNull(); // only one inside the window
  expect(sizeGrowth([s("2026-09-01", 1), s("2026-10-12", 3), s("2026-11-01", 4), s("2026-11-11", 9)])).toEqual({
    delta: 6,
    days: 30,
    recent: [s("2026-10-12", 3), s("2026-11-01", 4), s("2026-11-11", 9)],
  });
  expect(sizeGrowth([s("2026-11-09", 9), s("2026-11-11", 7)])?.delta).toBe(-2);
});
