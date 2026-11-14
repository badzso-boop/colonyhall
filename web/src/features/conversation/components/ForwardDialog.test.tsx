import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import { conversationKeys, type Message } from "../api";
import { ForwardDialog } from "./ForwardDialog";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const channels = [
  { id: "secret", kind: "private", name: "secret", is_member: true, unread: 0, mentions: 0 },
  { id: "general", kind: "public", name: "general", is_member: true, unread: 0, mentions: 0 },
  { id: "old", kind: "public", name: "old", is_member: true, unread: 0, mentions: 0, archived_at: "2026-01-01T00:00:00Z" },
];
const msg = { id: "m1", channel_id: "secret", author_id: "me", kind: "text", body: "terv", mentions: [], reply_count: 0, created_at: "" } as Message;

test("forwarding out of a private channel asks first, then posts to the chosen conversation", async () => {
  const posts: Array<{ path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      posts.push({ path: new URL(req.url).pathname, body: await req.clone().json().catch(() => null) });
      return new Response(JSON.stringify({ message: { ...msg, id: "m2", channel_id: "general" } }), { status: 201, headers: { "Content-Type": "application/json" } });
    }),
  );
  const qc = new QueryClient();
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "" });
  qc.setQueryData(identityKeys.actors, new Map([[me.id, me]]));
  qc.setQueryData(conversationKeys.channels, channels);
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ForwardDialog message={msg} onClose={onClose} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  const list = screen.getByRole("listbox", { name: "Hová" });
  expect(list.textContent).not.toContain("old");
  fireEvent.click(screen.getByRole("option", { name: /general/ }));
  expect(screen.getByRole("note").textContent).toContain("secret");
  fireEvent.change(screen.getByRole("textbox", { name: /Megjegyzés/ }), { target: { value: "@anna nézd" } });
  fireEvent.click(screen.getByRole("button", { name: "Mégis továbbítom" }));
  await waitFor(() => expect(posts.some((p) => p.path === "/api/v1/messages/m1/forward")).toBe(true));
  expect(posts.find((p) => p.path === "/api/v1/messages/m1/forward")?.body).toMatchObject({ channel_id: "general", comment: "@anna nézd" });
  await waitFor(() => expect(onClose).toHaveBeenCalled());
  vi.unstubAllGlobals();
});
