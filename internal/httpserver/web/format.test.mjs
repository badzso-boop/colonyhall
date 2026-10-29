import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import "./format.js";

const { dayKey, messageTime, fullTime, dayLabel, feedItems, sessionContext } = globalThis.ColonyFormat;
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const indexHtml = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");

// Local-time dates, so the tests hold in any time zone.
const now = new Date(2026, 9, 29, 15, 30, 0);
const words = { today: "Ma", yesterday: "Tegnap" };

test("dayKey is the local calendar day", () => {
  assert.equal(dayKey(new Date(2026, 9, 29, 0, 0, 1)), "2026-10-29");
  assert.equal(dayKey(new Date(2026, 9, 28, 23, 59, 59)), "2026-10-28");
  assert.equal(dayKey(new Date(2026, 0, 5, 12)), "2026-01-05");
  assert.equal(dayKey("not a date"), "");
});

test("messageTime shows the date with the time, and the year only for another year", () => {
  const sameYear = messageTime(new Date(2026, 9, 28, 9, 5, 7), now, "en-GB");
  assert.match(sameYear, /28/);
  assert.match(sameYear, /Oct/);
  assert.match(sameYear, /09:05:07/);
  assert.doesNotMatch(sameYear, /2026/);

  const otherYear = messageTime(new Date(2025, 11, 31, 23, 0, 0), now, "en-GB");
  assert.match(otherYear, /2025/);
  assert.match(otherYear, /31/);

  const hu = messageTime(new Date(2026, 9, 28, 9, 5, 7), now, "hu-HU");
  assert.match(hu, /okt/);
  assert.match(hu, /09:05:07/);
  assert.equal(messageTime("garbage", now, "hu-HU"), "");
});

test("fullTime names the weekday and the year", () => {
  const s = fullTime(new Date(2026, 9, 27, 8, 0, 0), "en-GB");
  assert.match(s, /Tuesday/);
  assert.match(s, /2026/);
  assert.match(s, /October/);
});

test("dayLabel says today and yesterday by word, other days by date and weekday", () => {
  assert.equal(dayLabel(new Date(2026, 9, 29, 0, 0, 0), now, "en-GB", words), "Ma");
  assert.equal(dayLabel(new Date(2026, 9, 28, 23, 59, 0), now, "en-GB", words), "Tegnap");
  const older = dayLabel(new Date(2026, 9, 27, 12, 0, 0), now, "en-GB", words);
  assert.match(older, /Tuesday/);
  assert.match(older, /27 October/);
  assert.doesNotMatch(older, /2026/);
  assert.match(dayLabel(new Date(2025, 9, 27, 12), now, "en-GB", words), /2025/);
  // Yesterday across a month boundary.
  const firstOfMonth = new Date(2026, 10, 1, 10, 0, 0);
  assert.equal(dayLabel(new Date(2026, 9, 31, 22, 0, 0), firstOfMonth, "en-GB", words), "Tegnap");
});

test("feedItems puts one separator before the first message of each day", () => {
  const msgs = [
    { id: "a", created_at: new Date(2026, 9, 27, 10).toISOString() },
    { id: "b", created_at: new Date(2026, 9, 27, 18).toISOString() },
    { id: "c", created_at: new Date(2026, 9, 28, 0, 0, 1).toISOString() },
    { id: "d", created_at: new Date(2026, 9, 29, 9).toISOString() },
    { id: "e", created_at: new Date(2026, 9, 29, 9, 1).toISOString() },
  ];
  const items = feedItems(msgs, now, "en-GB", words);
  const shape = items.map(i => (i.separator ? "|" + i.day : i.message.id));
  assert.deepEqual(shape, ["|2026-10-27", "a", "b", "|2026-10-28", "c", "|2026-10-29", "d", "e"]);
  assert.equal(items[3].label, "Tegnap");
  assert.equal(items[5].label, "Ma");
  assert.deepEqual(feedItems([], now, "en-GB", words), []);
});

test("sessionContext compares the measured context, not the run's summed usage", () => {
  // A run with many tool calls: 112k summed, but the context is 40k.
  const t = { last_run_tokens: 112000, context_tokens: 40000, max_session_tokens: 150000 };
  const c = sessionContext(t);
  assert.equal(c.text, "40k / 150k");
  assert.equal(c.known, true);
  assert.equal(c.near, false);
  assert.equal(c.runTokens, "112k");

  assert.equal(sessionContext({ last_run_tokens: 1, context_tokens: 125000, max_session_tokens: 150000 }).near, true);

  // Unknown context (older runs, Antigravity without a measurement).
  const u = sessionContext({ last_run_tokens: 200000, max_session_tokens: 150000 });
  assert.equal(u.text, "– / 150k");
  assert.equal(u.known, false);
  assert.equal(u.near, false);

  assert.equal(sessionContext({ last_run_tokens: 0, context_tokens: 5000, max_session_tokens: 0 }).text, "5k / ∞");
});

test("index.html uses the shared formatter for messages, separators and the overview", () => {
  assert.ok(indexHtml.includes('<script src="format.js"></script>'), "format.js is not loaded");
  assert.ok(indexHtml.includes("ColonyFormat.messageTime("), "renderMsg does not show the date");
  assert.ok(indexHtml.includes("ColonyFormat.feedItems("), "renderFeed does not add day separators");
  assert.ok(indexHtml.includes("function appendFeedMsg("), "live messages do not go through appendFeedMsg");
  assert.ok(indexHtml.includes("msgs.set(m.id, m); appendFeedMsg(m);"), "a live message bypasses the day separator");
  assert.equal(indexHtml.split("appendChild(renderMsg(").length - 1, 2, "a message is appended outside renderFeed/appendFeedMsg");
  assert.ok(indexHtml.includes("ColonyFormat.sessionContext("), "the overview still shows the summed usage as context");
  assert.ok(indexHtml.includes('today: "Ma"') && indexHtml.includes('today: "Today"'), "today label missing");
  assert.ok(indexHtml.includes('yesterday: "Tegnap"') && indexHtml.includes('yesterday: "Yesterday"'), "yesterday label missing");
});
