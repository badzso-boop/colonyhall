import test from "node:test";
import assert from "node:assert/strict";
import "./mention.js";

const { fuzzyScore, rankMentions } = globalThis.ColonyMention;

test("fuzzyScore: prefix match vs word-start vs subsequence", () => {
  // Prefix match
  const scorePrefix = fuzzyScore("norbi", "nor");
  assert.ok(scorePrefix >= 1000, `expected prefix score >= 1000, got ${scorePrefix}`);

  // Word-start match
  const scoreWordStart = fuzzyScore("szakdoga-claude", "claude");
  assert.ok(scoreWordStart >= 800 && scoreWordStart < 1000, `expected word-start score >= 800 and < 1000, got ${scoreWordStart}`);

  // Subsequence match
  const scoreSubseq = fuzzyScore("visegrad-agy", "vsg");
  assert.ok(scoreSubseq > 0 && scoreSubseq < 800, `expected subsequence score > 0 and < 800, got ${scoreSubseq}`);

  // No match
  const scoreNoMatch = fuzzyScore("norbi", "xyz");
  assert.equal(scoreNoMatch, 0);

  // Prefix beats word-start, word-start beats subsequence
  assert.ok(scorePrefix > scoreWordStart);
  assert.ok(scoreWordStart > scoreSubseq);
});

test("rankMentions: channel members ranked first", () => {
  const actors = [
    { id: "u1", handle: "norbi", display_name: "Norbert", kind: "human" },
    { id: "u2", handle: "nato-claude", display_name: "NATO Claude", kind: "agent", host_name: "laptop" },
    { id: "u3", handle: "szakdoga-claude", display_name: "Szakdoga Claude", kind: "agent", host_name: "desktop" },
    { id: "u4", handle: "claude-eval", display_name: "Claude Eval", kind: "agent", host_name: "server" },
  ];

  // u3 is a member of the current channel, others are not
  const memberIds = new Set(["u3"]);

  const res = rankMentions("claude", actors, memberIds);
  assert.equal(res.length, 3);
  // Member u3 (szakdoga-claude) must come first even if u4 has exact prefix match "claude-"
  assert.equal(res[0].id, "u3");
  // Non-members come next
  assert.ok(res.slice(1).some(a => a.id === "u2"));
  assert.ok(res.slice(1).some(a => a.id === "u4"));
});

test("rankMentions: excludes halted actors", () => {
  const actors = [
    { id: "u1", handle: "active-agent", display_name: "Active", kind: "agent", halted: false },
    { id: "u2", handle: "halted-agent", display_name: "Halted", kind: "agent", halted: true },
  ];

  const res = rankMentions("agent", actors, new Set());
  assert.equal(res.length, 1);
  assert.equal(res[0].id, "u1");
});

test("rankMentions: at most 8 items returned", () => {
  const actors = [];
  for (let i = 0; i < 20; i++) {
    actors.push({
      id: `a-${i}`,
      handle: `agent-${i}`,
      display_name: `Agent ${i}`,
      kind: "agent"
    });
  }

  const res = rankMentions("agent", actors, new Set());
  assert.equal(res.length, 8);
});

test("rankMentions: matches on display name too", () => {
  const actors = [
    { id: "u1", handle: "john_d", display_name: "Kovács János", kind: "human" },
    { id: "u2", handle: "peter_k", display_name: "Nagy Péter", kind: "human" },
  ];

  const res = rankMentions("jános", actors, new Set());
  assert.equal(res.length, 1);
  assert.equal(res[0].id, "u1");
});

test("rankMentions: empty query returns all up to 8 with members first", () => {
  const actors = [
    { id: "u1", handle: "alice", display_name: "Alice", kind: "human" },
    { id: "u2", handle: "bob", display_name: "Bob", kind: "human" },
    { id: "u3", handle: "charlie", display_name: "Charlie", kind: "human" },
  ];

  const memberIds = new Set(["u2"]);
  const res = rankMentions("", actors, memberIds);
  assert.equal(res.length, 3);
  assert.equal(res[0].id, "u2"); // member bob first
});
