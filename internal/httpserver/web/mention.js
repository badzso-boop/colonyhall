// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// ColonyMention: mention autocomplete fuzzy matching and ranking.
(function (global) {
  function escapeRegExp(s) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  function fuzzyScore(target, query) {
    if (!query) return 1;
    if (!target) return 0;
    const t = target.toLowerCase();
    const q = query.toLowerCase();

    // Exact match
    if (t === q) {
      return 1200;
    }

    // Exact prefix match
    if (t.startsWith(q)) {
      return 1000 + (100 - Math.min(100, t.length));
    }

    // Word-start match (after space, dash, underscore, dot)
    const wordBoundaryRe = new RegExp(`(?:^|[\\s_\\-.])${escapeRegExp(q)}`, "i");
    if (wordBoundaryRe.test(t)) {
      return 800 + (100 - Math.min(100, t.length));
    }

    // Subsequence match
    let tIdx = 0;
    let qIdx = 0;
    let gaps = 0;
    while (tIdx < t.length && qIdx < q.length) {
      if (t[tIdx] === q[qIdx]) {
        qIdx++;
      } else {
        gaps++;
      }
      tIdx++;
    }

    if (qIdx === q.length) {
      return 400 - Math.min(200, gaps);
    }

    return 0;
  }

  function rankMentions(query, actorsList, memberIds) {
    const q = (query || "").trim().toLowerCase();
    const mSet = memberIds instanceof Set ? memberIds : new Set(Array.isArray(memberIds) ? memberIds : []);
    const candidates = [];

    for (const a of actorsList || []) {
      if (a.halted) continue;

      const isMember = mSet.has(a.id);
      const handle = a.handle || "";
      const name = a.display_name || "";

      if (!q) {
        const score = isMember ? 10000 : 0;
        candidates.push({ actor: a, score, isMember });
        continue;
      }

      const handleScore = fuzzyScore(handle, q);
      const nameScore = fuzzyScore(name, q);
      const baseScore = Math.max(handleScore, nameScore);

      if (baseScore <= 0) continue;

      const score = (isMember ? 10000 : 0) + baseScore;
      candidates.push({ actor: a, score, isMember });
    }

    candidates.sort((x, y) => {
      if (y.score !== x.score) return y.score - x.score;
      return (x.actor.handle || "").localeCompare(y.actor.handle || "");
    });

    return candidates.slice(0, 8).map(c => c.actor);
  }

  const ColonyMention = {
    fuzzyScore,
    rankMentions,
  };

  if (typeof module !== "undefined" && module.exports) {
    module.exports = ColonyMention;
  }
  global.ColonyMention = ColonyMention;
})(typeof globalThis !== "undefined" ? globalThis : this);
