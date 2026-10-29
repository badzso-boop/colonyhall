import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const indexHtml = fs.readFileSync(path.join(__dirname, "index.html"), "utf8");

test("the DM reply link comes from the server's answers field, never from the message body", () => {
  // A body marker could be typed by any agent and is read back by agents in
  // thread context; the link must come from the checked answers field.
  assert.ok(!indexHtml.includes("dm_reply"), "index.html still parses a [dm_reply:...] body marker");
  assert.ok(indexHtml.includes("if (m.answers) {"), "renderMsg does not use m.answers");
  assert.ok(indexHtml.includes("actors[m.answers.author_id]"), "the author handle is not resolved from answers.author_id");
});

test("index.html contains reply routing and rate limit I18N keys for hu and en", () => {
  assert.ok(indexHtml.includes("fromThread: (ch) => `↩ a #${ch} szálból`"), "Hungarian fromThread missing");
  assert.ok(indexHtml.includes("fromThread: (ch) => `↩ from #${ch} thread`"), "English fromThread missing");
  assert.ok(indexHtml.includes("replyToDM: (agent) => `↪ válasz @${agent} DM-jére`"), "Hungarian replyToDM missing");
  assert.ok(indexHtml.includes("replyToDM: (agent) => `↪ reply to @${agent}'s DM`"), "English replyToDM missing");
  assert.ok(indexHtml.includes('claude5h: "Claude 5 óra:"'), "Hungarian claude5h missing");
  assert.ok(indexHtml.includes('claude5h: "Claude 5h:"'), "English claude5h missing");
  assert.ok(indexHtml.includes('claudeWeekly: "heti"'), "Hungarian claudeWeekly missing");
  assert.ok(indexHtml.includes('claudeWeekly: "weekly"'), "English claudeWeekly missing");
  assert.ok(indexHtml.includes('claudePausedBadge: "⏸ szünetel"'), "Hungarian claudePausedBadge missing");
  assert.ok(indexHtml.includes('claudePausedBadge: "⏸ paused"'), "English claudePausedBadge missing");
  assert.ok(indexHtml.includes('resetsAtPrefix: "újraindul"'), "Hungarian resetsAtPrefix missing");
  assert.ok(indexHtml.includes('resetsAtPrefix: "resets at"'), "English resetsAtPrefix missing");
  assert.ok(indexHtml.includes('claudeLimitsLabel: "Claude limitek:"'), "Hungarian claudeLimitsLabel missing");
  assert.ok(indexHtml.includes('claudeLimitsLabel: "Claude limits:"'), "English claudeLimitsLabel missing");
});

test("index.html renders msg-origin-link and msg-dm-reply-link in renderMsg", () => {
  assert.ok(indexHtml.includes("msg-origin-link"), "msg-origin-link container missing");
  assert.ok(indexHtml.includes("msg-dm-reply-link"), "msg-dm-reply-link container missing");
  assert.ok(indexHtml.includes("renderClaudeLimitsElement(h.report.claude_limits)"), "hosts panel limits call missing");
  assert.ok(indexHtml.includes("renderClaudeLimitsElement(asgn.claude_limits)"), "overview panel limits call missing");
});

test("renderClaudeLimits logic formats utilization, resets_at, and pause status", () => {
  function formatLimits(limits, lang = "hu") {
    if (!limits) return null;
    const fh = limits.five_hour;
    const sd = limits.seven_day;
    if (!fh && !sd && !limits.paused) return null;

    const warnPct = limits.rate_limit_warn || 80;
    const reservePct = limits.rate_limit_reserve || 90;

    const parts = [];
    let isWarn = false;
    let isPaused = !!limits.paused;

    const labels = lang === "hu"
      ? { c5h: "Claude 5 óra:", resets: "újraindul", weekly: "heti", paused: "⏸ szünetel" }
      : { c5h: "Claude 5h:", resets: "resets at", weekly: "weekly", paused: "⏸ paused" };

    if (fh && fh.utilization != null) {
      const pct = Math.round(fh.utilization * 100);
      if (pct >= warnPct) isWarn = true;
      if (pct >= reservePct) isPaused = true;
      let s = `${labels.c5h} ${pct}%`;
      if (fh.resets_at) {
        const d = new Date(fh.resets_at);
        if (!isNaN(d.getTime())) {
          // Format as UTC HH:MM for deterministic test
          const hours = String(d.getUTCHours()).padStart(2, "0");
          const mins = String(d.getUTCMinutes()).padStart(2, "0");
          s += ` · ${labels.resets} ${hours}:${mins}`;
        }
      }
      parts.push(s);
    }

    if (sd && sd.utilization != null) {
      const pct = Math.round(sd.utilization * 100);
      if (pct >= warnPct) isWarn = true;
      if (pct >= reservePct) isPaused = true;
      let s = `${labels.weekly} ${pct}%`;
      parts.push(s);
    }

    return {
      text: parts.join(" · "),
      isWarn,
      isPaused
    };
  }

  // Normal utilization (72% 5h, 59% weekly)
  const normal = formatLimits({
    five_hour: { utilization: 0.72, resets_at: "2026-10-29T18:00:00Z", status: "allowed" },
    seven_day: { utilization: 0.59, status: "allowed" },
    rate_limit_reserve: 90,
    rate_limit_warn: 80,
  });
  assert.equal(normal.text, "Claude 5 óra: 72% · újraindul 18:00 · heti 59%");
  assert.equal(normal.isWarn, false);
  assert.equal(normal.isPaused, false);

  // Warning utilization (82% 5h)
  const warning = formatLimits({
    five_hour: { utilization: 0.82, resets_at: "2026-10-29T18:00:00Z", status: "allowed" },
    seven_day: { utilization: 0.59, status: "allowed" },
  });
  assert.equal(warning.isWarn, true);
  assert.equal(warning.isPaused, false);

  // Reserve reached (91% 5h)
  const reserve = formatLimits({
    five_hour: { utilization: 0.91, resets_at: "2026-10-29T18:00:00Z", status: "allowed" },
    seven_day: { utilization: 0.59, status: "allowed" },
  });
  assert.equal(reserve.isPaused, true);
});
