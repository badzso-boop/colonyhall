// ColonyFormat: message dates, day separators and the session context label.
// Pure functions, so they are tested in Node (format.test.mjs).
(function (global) {
  // dayKey is the viewer's local calendar day of d, e.g. "2026-10-29".
  function dayKey(d) {
    const date = d instanceof Date ? d : new Date(d);
    if (isNaN(date.getTime())) return "";
    const m = date.getMonth() + 1;
    const day = date.getDate();
    return `${date.getFullYear()}-${m < 10 ? "0" : ""}${m}-${day < 10 ? "0" : ""}${day}`;
  }

  function addDays(d, n) {
    const out = new Date(d.getTime());
    out.setDate(out.getDate() + n);
    return out;
  }

  // messageTime is the date and time shown next to a message: always with
  // the day, and with the year only when it is not the current one.
  function messageTime(d, now, locale) {
    const date = d instanceof Date ? d : new Date(d);
    if (isNaN(date.getTime())) return "";
    const opts = { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" };
    if (date.getFullYear() !== now.getFullYear()) opts.year = "numeric";
    return date.toLocaleString(locale, opts);
  }

  // fullTime is the tooltip: the complete date with weekday and time.
  function fullTime(d, locale) {
    const date = d instanceof Date ? d : new Date(d);
    if (isNaN(date.getTime())) return "";
    return date.toLocaleString(locale, {
      year: "numeric", month: "long", day: "numeric", weekday: "long",
      hour: "2-digit", minute: "2-digit", second: "2-digit",
    });
  }

  // dayLabel names a day separator: "today" and "yesterday" by word, any
  // other day by its full date and weekday.
  function dayLabel(d, now, locale, words) {
    const date = d instanceof Date ? d : new Date(d);
    if (isNaN(date.getTime())) return "";
    const key = dayKey(date);
    if (key === dayKey(now)) return words.today;
    if (key === dayKey(addDays(now, -1))) return words.yesterday;
    const opts = { month: "long", day: "numeric", weekday: "long" };
    if (date.getFullYear() !== now.getFullYear()) opts.year = "numeric";
    return date.toLocaleDateString(locale, opts);
  }

  // feedItems interleaves day separators with messages in the given order:
  // a separator precedes the first message of every new local day.
  function feedItems(messages, now, locale, words) {
    const out = [];
    let last = null;
    for (const m of messages) {
      const key = dayKey(m.created_at);
      if (key && key !== last) {
        out.push({ separator: true, day: key, label: dayLabel(m.created_at, now, locale, words) });
        last = key;
      }
      out.push({ separator: false, message: m });
    }
    return out;
  }

  // sessionContext describes a thread's session for the agent overview: the
  // measured context size against the session limit. The run's summed usage
  // (last_run_tokens) is not the context, so it is only shown as detail.
  function sessionContext(t) {
    const k = (n) => Math.round(n / 1000) + "k";
    const ctx = t.context_tokens > 0 ? t.context_tokens : 0;
    const max = t.max_session_tokens > 0 ? t.max_session_tokens : 0;
    return {
      known: ctx > 0,
      near: ctx > 0 && max > 0 && ctx >= max * 0.8,
      text: `${ctx > 0 ? k(ctx) : "–"} / ${max > 0 ? k(max) : "∞"}`,
      runTokens: t.last_run_tokens > 0 ? k(t.last_run_tokens) : "0k",
    };
  }

  const ColonyFormat = { dayKey, messageTime, fullTime, dayLabel, feedItems, sessionContext };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = ColonyFormat;
  }
  global.ColonyFormat = ColonyFormat;
})(typeof globalThis !== "undefined" ? globalThis : this);
