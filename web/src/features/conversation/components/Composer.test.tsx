import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import { Toaster, useToasts } from "@/shared/ui";
import type { AttachmentTray } from "../attachments";
import { Composer } from "./Composer";

const actor = (id: string, kind: "human" | "agent", handle: string) => ({
  id, kind, handle, display_name: handle, role: "member", state: "active", created_at: "2026-10-30T00:00:00Z",
});
const me = actor("me", "human", "norbi");
const agent = actor("ag", "agent", "fake-agent");

const question = (id: string, number: number, text: string) => ({
  id, number, formatted_number: `Q-${number}`, workspace_id: "w", asked_by: "ag", asked_by_handle: "fake-agent", to_human: true,
  channel_id: "c1", channel_name: "general", thread_root_id: "t", text, options: [], state: "open", created_at: "2026-10-30T00:00:00Z",
});
const questions = [question("q12", 12, "Melyik adatbázis?"), question("q3", 3, "Mehet a deploy?")];

const tray: AttachmentTray = { items: [], add: vi.fn(), remove: vi.fn(), retry: vi.fn(), clear: vi.fn(), restore: vi.fn(), busy: false, fileIds: [], failed: false };

type Draft = { channel_id: string; thread_root_id?: string; body: string; attachments: []; updated_at: string };

function setup(drafts: Draft[] = [], thread?: { rootId: string; channel: string }, role = "member") {
  const qc = new QueryClient();
  qc.setQueryData(identityKeys.actors, new Map([[me.id, me], [agent.id, agent]]));
  qc.setQueryData(identityKeys.session, { actor: { ...me, role }, csrf_token: "" });
  const sent: unknown[] = [];
  const draftCalls: Array<{ method: string; body?: unknown; query?: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (path.endsWith("/me/drafts")) {
        if (req.method === "GET") return new Response(JSON.stringify({ drafts }), { headers: { "Content-Type": "application/json" } });
        draftCalls.push({ method: req.method, body: await req.clone().json().catch(() => undefined), query: new URL(req.url).search });
        return new Response(null, { status: 204 });
      }
      if (req.method === "PUT" || req.method === "DELETE") {
        sent.push({ method: req.method, path, body: await req.clone().json().catch(() => null) });
        return new Response(null, { status: 204 });
      }
      if (req.method === "POST") sent.push({ path: new URL(req.url).pathname, body: await req.clone().json().catch(() => null) });
      if (req.method === "GET" && new URL(req.url).pathname.endsWith("/questions")) {
        const state = new URL(req.url).searchParams.get("state");
        return new Response(JSON.stringify({ questions: state === "open" ? questions : [] }), { headers: { "Content-Type": "application/json" } });
      }
      if (req.method === "GET") return new Response(JSON.stringify({ actors: [me, agent] }), { headers: { "Content-Type": "application/json" } });
      return new Response(JSON.stringify({ message: { id: "m1" } }), { status: 201, headers: { "Content-Type": "application/json" } });
    }),
  );
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
      <Composer channelId="c1" threadRootId={thread?.rootId} alsoToChannel={thread?.channel} placeholder="Üzenet" memberIds={new Set(["ag"])} tray={tray} />
      </MemoryRouter>
      <Toaster closeLabel="Bezárás" />
    </QueryClientProvider>,
  );
  return { sent, draftCalls, box: screen.getByRole("textbox", { name: "Üzenet" }) as HTMLTextAreaElement };
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  useToasts.setState({ items: [] });
});

test("@ suggests members and inserts the chosen handle", () => {
  const { box } = setup();
  fireEvent.change(box, { target: { value: "szia @fa", selectionStart: 8 } });
  expect(screen.getByRole("listbox", { name: "Megemlíthető résztvevők" })).toBeTruthy();
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe("szia @fake-agent ");
});

test("/ lists the commands", () => {
  const { box } = setup();
  fireEvent.change(box, { target: { value: "/da", selectionStart: 3 } });
  const list = screen.getByRole("listbox", { name: "Parancsok" });
  expect(list.textContent).toContain("/daily 09:00");
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe("/daily ");
});

test("Enter sends the message; Shift+Enter does not", async () => {
  const { box, sent } = setup();
  fireEvent.change(box, { target: { value: "hello", selectionStart: 5 } });
  fireEvent.keyDown(box, { key: "Enter", shiftKey: true });
  expect(sent).toHaveLength(0);
  await act(async () => {
    fireEvent.keyDown(box, { key: "Enter" });
  });
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({ path: "/api/v1/channels/c1/messages", body: { body: "hello" } });
  await waitFor(() => expect(box.value).toBe(""));
  expect(screen.getByRole("button", { name: "Visszavonás" })).toBeTruthy();
});

test("a malformed command is explained, not sent", async () => {
  const { box, sent } = setup();
  fireEvent.change(box, { target: { value: "/daily soon", selectionStart: 11 } });
  await act(async () => {
    fireEvent.keyDown(box, { key: "Enter" });
  });
  expect(sent).toHaveLength(0);
  expect(screen.getByRole("status").textContent).toContain("/schedule");
});

test("/answer suggests the open question numbers", async () => {
  const { box } = setup();
  fireEvent.change(box, { target: { value: "/answer Q-1", selectionStart: 11 } });
  const list = await screen.findByRole("listbox", { name: "Nyitott kérdések" });
  expect(list.textContent).toContain("Q-12");
  expect(list.textContent).toContain("Melyik adatbázis?");
  expect(list.textContent).not.toContain("Q-3");
  fireEvent.keyDown(box, { key: "Enter" });
  expect(box.value).toBe("/answer Q-12 ");
  expect(screen.queryByRole("listbox")).toBeNull();
});

test("the draft comes back from the server when the conversation opens", async () => {
  const { box } = setup([{ channel_id: "c1", body: "félkész gondolat", attachments: [], updated_at: "2026-11-14T10:00:00Z" }]);
  await waitFor(() => expect(box.value).toBe("félkész gondolat"));
});

test("a newer local copy (typed offline) wins over the server's", async () => {
  localStorage.setItem("colony.draft.c1:", JSON.stringify({ body: "offline", files: [], at: "2026-11-14T11:00:00Z" }));
  const { box } = setup([{ channel_id: "c1", body: "régi", attachments: [], updated_at: "2026-11-14T10:00:00Z" }]);
  await waitFor(() => expect(box.value).toBe("offline"));
});

test("typing saves the draft after a pause, and sending removes it", async () => {
  const { box, draftCalls } = setup();
  await waitFor(() => expect(box).toBeTruthy());
  await act(async () => {
    await new Promise((r) => setTimeout(r, 20));
  });
  fireEvent.change(box, { target: { value: "majdnem kész", selectionStart: 12 } });
  expect(draftCalls).toHaveLength(0);
  expect(localStorage.getItem("colony.draft.c1:")).toContain("majdnem kész");
  await waitFor(() => expect(draftCalls).toHaveLength(1), { timeout: 2500 });
  expect(draftCalls[0]).toMatchObject({ method: "PUT", body: { channel_id: "c1", body: "majdnem kész" } });
  await act(async () => {
    fireEvent.keyDown(box, { key: "Enter" });
  });
  await waitFor(() => expect(draftCalls.at(-1)).toMatchObject({ method: "DELETE", query: "?channel_id=c1" }));
  expect(localStorage.getItem("colony.draft.c1:")).toBeNull();
});

test("a thread reply can also go to the channel, as one flagged message", async () => {
  const { box, sent } = setup([], { rootId: "r1", channel: "general" });
  fireEvent.click(screen.getByRole("checkbox", { name: "Küldés a #general csatornába is" }));
  fireEvent.change(box, { target: { value: "kész", selectionStart: 4 } });
  await act(async () => {
    fireEvent.keyDown(box, { key: "Enter" });
  });
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({ body: { body: "kész", thread_root_id: "r1", also_in_channel: true } });
  await waitFor(() => expect((screen.getByRole("checkbox", { name: "Küldés a #general csatornába is" }) as HTMLInputElement).checked).toBe(false));
});

async function submit(box: HTMLTextAreaElement, value: string) {
  fireEvent.change(box, { target: { value, selectionStart: value.length } });
  await act(async () => {
    fireEvent.keyDown(box, { key: "Enter" });
  });
}

test("an unknown command is refused with a hint and kept, never sent", async () => {
  const { box, sent } = setup();
  await submit(box, "/frobnicate now");
  expect(sent).toHaveLength(0);
  expect(screen.getByRole("status").textContent).toContain("/frobnicate");
  expect(box.value).toBe("/frobnicate now");
});

test("//text sends a message that starts with /", async () => {
  const { box, sent } = setup();
  await submit(box, "//pin is a command");
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({ path: "/api/v1/channels/c1/messages", body: { body: "/pin is a command" } });
});

test("/notify sets the level here instead of sending", async () => {
  const { box, sent } = setup();
  await submit(box, "/notify all");
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({ method: "PUT", path: "/api/v1/channels/c1/notify", body: { level: "all" } });
  await waitFor(() => expect(box.value).toBe(""));
});

test("/remind keeps a text with a time as a saved reminder", async () => {
  const { box, sent } = setup();
  await submit(box, "/remind 20m nézd meg a buildet");
  await waitFor(() => expect(sent).toHaveLength(1));
  expect(sent[0]).toMatchObject({ path: "/api/v1/me/saved", body: { note: "nézd meg a buildet" } });
  expect(typeof (sent[0] as { body: { remind_at: string } }).body.remind_at).toBe("string");
});

test("/help lists every command", async () => {
  const { box } = setup();
  await submit(box, "/help");
  await waitFor(() => expect(box.value).toBe("/"));
  const list = screen.getByRole("listbox", { name: "Parancsok" });
  expect(list.textContent).toContain("/remind");
  expect(list.textContent).toContain("/search");
});

test("/emergency is neither listed nor run for a member, and only opens a dialog for an admin", async () => {
  const { box, sent } = setup();
  fireEvent.change(box, { target: { value: "/em", selectionStart: 3 } });
  expect(screen.queryByRole("listbox", { name: "Parancsok" })).toBeNull();
  await submit(box, "/emergency");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(sent).toHaveLength(0);
});

test("/emergency opens the press-and-hold dialog for an admin; Enter stops nothing", async () => {
  const { box, sent } = setup([], undefined, "admin");
  fireEvent.change(box, { target: { value: "/em", selectionStart: 3 } });
  expect(screen.getByRole("listbox", { name: "Parancsok" }).textContent).toContain("/emergency");
  await submit(box, "/emergency");
  await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
  expect(sent).toHaveLength(0);
});
