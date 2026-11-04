import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Actor, Channel } from "../model";
import { conversationKeys } from "../api";
import { ChannelSettingsDialog } from "./ChannelSettingsDialog";

const actor = (id: string, kind: string, handle: string) => ({ id, kind, handle, display_name: handle, role: "member", state: "active", created_at: "" }) as Actor;
const actors = new Map([actor("me", "human", "norbi"), actor("a1", "agent", "trader"), actor("h2", "human", "anna")].map((a) => [a.id, a]));
const channel = { id: "c1", kind: "public", name: "ops", description: "Deploys", is_member: true, unread: 0, mentions: 0 } as Channel;

let requests: { method: string; path: string; body: unknown }[] = [];

function setup(admin: boolean, tab: "altalanos" | "tagok" = "altalanos") {
  requests = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const path = new URL(req.url).pathname;
      requests.push({ method: req.method, path, body: req.method === "GET" ? null : await req.text() });
      if (path.endsWith("/members")) return Response.json({ members: [{ actor_id: "me", handle: "norbi", kind: "human", can_write: true }, { actor_id: "a1", handle: "trader", kind: "agent", can_write: false }] });
      return new Response(null, { status: 204 });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(conversationKeys.channels, [channel]);
  const onTab = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ChannelSettingsDialog channel={channel} actors={actors} admin={admin} tab={tab} onTab={onTab} onClose={() => {}} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { onTab };
}

afterEach(() => vi.unstubAllGlobals());

test("admins get all four tabs and edit the description", async () => {
  setup(true);
  for (const name of ["Általános", "Tagok", "Tömörítés", "Archiválás"]) expect(screen.getByRole("button", { name })).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: /Leírás/ }), { target: { value: "Deploys and incidents" } });
  expect(screen.getByText("21/250")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(requests.some((r) => r.method === "PATCH" && r.body === JSON.stringify({ description: "Deploys and incidents" }))).toBe(true));
});

test("non-admins see the read-only details: no archive, no save", () => {
  setup(false);
  expect(screen.getByText("#ops adatai")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Archiválás" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Mentés" })).toBeNull();
  expect(screen.queryByRole("textbox", { name: /Leírás/ })).toBeNull();
});

test("removing an agent warns that its work stops", async () => {
  setup(true, "tagok");
  expect(await screen.findByText("@trader")).toBeTruthy();
  fireEvent.click(screen.getAllByRole("button", { name: "Eltávolítás" })[1]!);
  expect(screen.getByText(/a futása leáll, a függő feladatai törlődnek/)).toBeTruthy();
  const dialogButtons = screen.getAllByRole("button", { name: "Eltávolítás" });
  fireEvent.click(dialogButtons[dialogButtons.length - 1]!);
  await waitFor(() => expect(requests.some((r) => r.method === "DELETE" && r.path.endsWith("/channels/c1/members/a1"))).toBe(true));
});

test("admins switch a member between read-only and read-write", async () => {
  setup(true, "tagok");
  const select = (await screen.findByRole("combobox", { name: "@trader jogosultsága" })) as HTMLSelectElement;
  expect(select.value).toBe("read");
  fireEvent.change(select, { target: { value: "write" } });
  await waitFor(() =>
    expect(requests.some((r) => r.method === "PATCH" && r.path.endsWith("/channels/c1/members/a1") && r.body === JSON.stringify({ can_write: true }))).toBe(true),
  );
});

test("members are added with the chosen access", async () => {
  setup(true, "tagok");
  await screen.findByText("@trader");
  fireEvent.click(screen.getByRole("button", { name: "Tagok hozzáadása…" }));
  fireEvent.click(screen.getByRole("checkbox", { name: /anna/ }));
  fireEvent.change(screen.getByRole("combobox", { name: "Jogosultság" }), { target: { value: "read" } });
  fireEvent.click(screen.getByRole("button", { name: "1 tag hozzáadása" }));
  await waitFor(() =>
    expect(requests.some((r) => r.method === "POST" && r.path.endsWith("/channels/c1/members") && r.body === JSON.stringify({ actor_ids: ["h2"], can_write: false }))).toBe(true),
  );
});

test("non-admins see who can only read", async () => {
  setup(false, "tagok");
  expect(await screen.findByText("Csak olvashat")).toBeTruthy();
  expect(screen.queryByRole("combobox")).toBeNull();
});
