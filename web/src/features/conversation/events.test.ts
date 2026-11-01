import { QueryClient } from "@tanstack/react-query";
import { dispatch } from "@/shared/api";
import { conversationKeys, type Message, type MessagePages } from "./api";
import { registerConversationEvents } from "./events";

const root: Message = { id: "root", channel_id: "c1", author_id: "me", kind: "text", body: "plan?", mentions: [], reply_count: 0, created_at: "2026-10-30T09:00:00Z" };

function created(id: string, extra: Record<string, unknown> = {}) {
  return {
    type: "event", seq: 1, event_type: "message.created", actor_id: "x", at: "",
    data: { message_id: id, channel_id: "c1", author_id: "agent", kind: "text", body: id, mentions: [], created_at: "2026-10-30T10:00:00Z", ...extra },
  };
}

test("a live message is appended once; a reply bumps its root and the open thread", () => {
  const qc = new QueryClient();
  qc.setQueryData<MessagePages>(conversationKeys.messages("c1"), { pages: [[root]], pageParams: [undefined] });
  qc.setQueryData(conversationKeys.thread("root"), { root, replies: [] });
  const off = registerConversationEvents(qc);

  dispatch(created("m2"));
  dispatch(created("m2")); // replayed
  dispatch(created("r1", { thread_root_id: "root" }));

  const pages = qc.getQueryData<MessagePages>(conversationKeys.messages("c1"))!;
  expect(pages.pages[0]!.map((m) => m.id)).toEqual(["root", "m2", "r1"]);
  expect(pages.pages[0]![0]).toMatchObject({ reply_count: 1, reply_participant_ids: ["agent"] });
  const thread = qc.getQueryData<{ root: Message; replies: Message[] }>(conversationKeys.thread("root"))!;
  expect(thread.replies.map((r) => r.id)).toEqual(["r1"]);
  expect(thread.root.reply_count).toBe(1);
  off();
});

test("channel edits refetch the sidebar and the members; a reply refetches Szálaim", () => {
  const qc = new QueryClient();
  const off = registerConversationEvents(qc);
  const spy = vi.spyOn(qc, "invalidateQueries");
  const ev = (event_type: string, data: Record<string, unknown>) => dispatch({ type: "event", seq: 2, event_type, actor_id: "x", at: "", data });

  ev("channel.updated", { channel_id: "c1", fields: ["description"] });
  ev("channel.archived", { channel_id: "c1", name: "ops" });
  const keys = spy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
  expect(keys).toContain(JSON.stringify(conversationKeys.channels));
  expect(keys).toContain(JSON.stringify(conversationKeys.members("c1")));

  spy.mockClear();
  dispatch(created("r2", { thread_root_id: "root" }));
  expect(spy.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey))).toContain(JSON.stringify(["conversation", "myThreads"]));
  off();
});
