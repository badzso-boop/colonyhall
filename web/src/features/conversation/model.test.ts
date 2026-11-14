import { buildSidebar, classifyConversation, conversationTitle, type Actor, type Channel } from "./model";

const actor = (id: string, kind: "human" | "agent", handle: string, display = handle): Actor => ({
  id, kind, handle, display_name: display, role: "member", state: "active", created_at: "2026-10-30T00:00:00Z",
});
const me = actor("me", "human", "norbi", "Norbi");
const anna = actor("anna", "human", "anna", "Anna Kovács");
const backend = actor("backend", "agent", "backend-claude");
const reviewer = actor("reviewer", "agent", "reviewer-claude");
const actors = new Map([me, anna, backend, reviewer].map((a) => [a.id, a]));

const ch = (id: string, kind: Channel["kind"], name: string, members?: string[], extra: Partial<Channel> = {}): Channel => ({
  id, kind, name, is_member: members ? members.includes("me") : true, unread: 0, mentions: 0, member_ids: members, ...extra,
});

test("classifyConversation covers every kind of conversation", () => {
  expect(classifyConversation(ch("1", "public", "general"), actors, "me")).toBe("channel");
  expect(classifyConversation(ch("2", "private", "ops"), actors, "me")).toBe("channel");
  expect(classifyConversation(ch("3", "dm", "x", ["me", "anna"]), actors, "me")).toBe("human-dm");
  expect(classifyConversation(ch("4", "dm", "x", ["me", "backend"]), actors, "me")).toBe("agent-dm");
  expect(classifyConversation(ch("5", "dm", "x", ["me", "anna", "backend"]), actors, "me")).toBe("agent-dm");
  expect(classifyConversation(ch("6", "dm", "x", ["backend", "reviewer"]), actors, "me")).toBe("agent-agent");
  expect(classifyConversation(ch("7", "dm", "x", ["me"]), actors, "me")).toBe("self");
  // A member no longer listed (e.g. removed): treated by what is known.
  expect(classifyConversation(ch("8", "dm", "x", ["me", "ghost"]), actors, "me")).toBe("human-dm");
  expect(classifyConversation(ch("9", "dm", "x", ["me", "ghost", "backend"]), actors, "me")).toBe("agent-dm");
});

test("conversationTitle names channels and DM peers", () => {
  expect(conversationTitle(ch("1", "public", "general"), actors, "me")).toBe("general");
  expect(conversationTitle(ch("3", "dm", "x", ["me", "anna"]), actors, "me")).toBe("Anna Kovács");
  expect(conversationTitle(ch("4", "dm", "x", ["me", "backend"]), actors, "me")).toBe("@backend-claude");
  expect(conversationTitle(ch("6", "dm", "x", ["backend", "reviewer"]), actors, "me")).toBe("@backend-claude ↔ @reviewer-claude");
  // The viewer's own DM, where reminders arrive (round 55): their name, not dm-….
  expect(conversationTitle(ch("7", "dm", "dm-0123456789abcdef", ["me"]), actors, "me")).toBe("Norbi");
});

test("buildSidebar: own sections first, each conversation once, then the fixed groups", () => {
  const channels = [
    ch("g", "public", "general"),
    ch("r", "public", "release"),
    ch("b", "public", "backend"),
    ch("dmA", "dm", "x", ["me", "anna"]),
    ch("dmB", "dm", "x", ["me", "backend"]),
    ch("aa", "dm", "x", ["backend", "reviewer"]),
  ];
  const sections = [{ id: "s1", name: "Kiadás", position: 0, collapsed: false, items: [{ channel_id: "r", position: 0 }, { channel_id: "missing", position: 1 }] }];
  const groups = buildSidebar(channels, sections, actors, "me");
  expect(groups.map((g) => [g.id, g.rows.map((r) => r.channel.id)])).toEqual([
    ["s1", ["r"]],
    ["channels", ["b", "g"]],
    ["dms", ["dmA"]],
    ["agentDms", ["dmB"]],
    ["agentAgent", ["aa"]],
  ]);
  expect(groups[4]!.rows[0]!.observed).toBe(true);
});

import { applyReply, mainStream } from "./model";
import type { Schemas } from "@/shared/api";

const msg = (id: string, extra: Partial<Schemas["Message"]> = {}): Schemas["Message"] => ({
  id, channel_id: "c", author_id: "me", kind: "text", body: id, mentions: [], reply_count: 0, created_at: "2026-10-30T09:00:00Z", ...extra,
});

test("applyReply bumps the root: count, time, the replier first, at most three", () => {
  const root = msg("root", { reply_count: 3, reply_participant_ids: ["a", "b", "c"] });
  const [after] = applyReply([root, msg("other")], msg("r", { thread_root_id: "root", author_id: "c", created_at: "2026-10-30T10:00:00Z" }));
  expect(after).toMatchObject({ reply_count: 4, last_reply_at: "2026-10-30T10:00:00Z", reply_participant_ids: ["c", "a", "b"] });
  const [fresh] = applyReply([msg("root")], msg("r", { thread_root_id: "root", author_id: "x" }));
  expect(fresh!.reply_participant_ids).toEqual(["x"]);
});

test("mainStream keeps replies out of channels but not out of DMs", () => {
  const list = [msg("a"), msg("b", { thread_root_id: "a" })];
  expect(mainStream(list, "public").map((m) => m.id)).toEqual(["a"]);
  expect(mainStream(list, "dm").map((m) => m.id)).toEqual(["a", "b"]);
});

test("the main stream keeps thread replies also sent to the channel", async () => {
  const { mainStream } = await import("./model");
  const base = { channel_id: "c", author_id: "a", kind: "text" as const, body: "", mentions: [], reply_count: 0, created_at: "" };
  const list = [
    { ...base, id: "r" },
    { ...base, id: "x", thread_root_id: "r" },
    { ...base, id: "y", thread_root_id: "r", also_in_channel: true },
  ];
  expect(mainStream(list, "public").map((m) => m.id)).toEqual(["r", "y"]);
  expect(mainStream(list, "dm").map((m) => m.id)).toEqual(["r", "x", "y"]);
});
