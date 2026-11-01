import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
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

const tray: AttachmentTray = { items: [], add: vi.fn(), remove: vi.fn(), retry: vi.fn(), clear: vi.fn(), busy: false, fileIds: [], failed: false };

function setup() {
  const qc = new QueryClient();
  qc.setQueryData(identityKeys.actors, new Map([[me.id, me], [agent.id, agent]]));
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "" });
  const sent: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
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
      <Composer channelId="c1" placeholder="Üzenet" memberIds={new Set(["ag"])} tray={tray} />
      <Toaster closeLabel="Bezárás" />
    </QueryClientProvider>,
  );
  return { sent, box: screen.getByRole("textbox", { name: "Üzenet" }) as HTMLTextAreaElement };
}

afterEach(() => {
  vi.unstubAllGlobals();
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
