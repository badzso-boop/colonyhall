// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { identityKeys, type Actor } from "@/features/identity";
import { useToasts } from "@/shared/ui";
import { conversationKeys } from "../api";
import { ConversationList } from "./ConversationList";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "admin", state: "active", created_at: "" } as Actor;
const channel = (id: string, name: string): Schemas["Channel"] =>
  ({ id, kind: "public", name, is_member: true, unread: 0, mentions: 0, member_ids: ["me"] }) as Schemas["Channel"];
const sections: Schemas["SidebarSection"][] = [
  { id: "s1", name: "Munka", position: 0, collapsed: false, items: [{ channel_id: "c1", position: 0 }] },
  { id: "s2", name: "Hobbi", position: 1, collapsed: false, items: [] },
];

type Call = { method: string; path: string; body: unknown };

function setup(fail = false) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const path = new URL(req.url).pathname.replace("/api/v1", "");
      const text = req.method === "GET" ? "" : await req.text();
      calls.push({ method: req.method, path, body: text ? JSON.parse(text) : null });
      if (req.method === "GET" && path === "/sidebar/sections") return Response.json({ sections });
      if (req.method === "GET" && path === "/channels") return Response.json({ channels: [channel("c1", "alpha"), channel("c2", "beta")] });
      if (fail) return Response.json({ title: "nope", status: 500 }, { status: 500 });
      if (req.method === "POST" && path === "/sidebar/sections") return Response.json({ id: "s3", name: "Új", position: 2, collapsed: false, items: [] }, { status: 201 });
      if (req.method === "PATCH") return Response.json({ ...sections[0], name: "Meló" });
      return new Response(null, { status: 204 });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  qc.setQueryData(identityKeys.actors, new Map([["me", me]]));
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "x" });
  qc.setQueryData(conversationKeys.sections, sections);
  qc.setQueryData(conversationKeys.channels, [channel("c1", "alpha"), channel("c2", "beta")]);
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ConversationList />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { calls, qc };
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  useToasts.setState({ items: [] });
});

const openMenu = (name: string) => fireEvent.pointerDown(screen.getByRole("button", { name }), { button: 0, ctrlKey: false, pointerType: "mouse" });

test("the row menu's Áthelyezés… moves a conversation into a section", async () => {
  const { calls } = setup();
  openMenu("beta műveletei");
  fireEvent.click(await screen.findByRole("menuitem", { name: "Áthelyezés…" }));
  const dialog = await screen.findByRole("dialog", { name: "Áthelyezés szakaszba" });
  fireEvent.click(within(dialog).getByRole("radio", { name: "Hobbi" }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Áthelyezés" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.path === "/sidebar/sections/s2/reorder")).toBe(true));
  expect(calls.find((c) => c.method === "PUT" && c.path === "/sidebar/sections/s2/channels/c2")?.body).toEqual({ position: 0 });
  expect(calls.find((c) => c.path === "/sidebar/sections/s2/reorder")?.body).toEqual({ channel_ids: ["c2"] });
});

test("moving back to the default group removes the item", async () => {
  const { calls } = setup();
  openMenu("alpha műveletei");
  fireEvent.click(await screen.findByRole("menuitem", { name: "Áthelyezés…" }));
  const dialog = await screen.findByRole("dialog");
  fireEvent.click(within(dialog).getByRole("radio", { name: "Az alapértelmezett csoportja" }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Áthelyezés" }));
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.path === "/sidebar/sections/s1/channels/c1")).toBe(true));
});

test("a refused drag is rolled back with a toast", async () => {
  const { calls, qc } = setup(true);
  const data = { setData: () => {}, dropEffect: "", effectAllowed: "" };
  const beta = screen.getByRole("link", { name: /beta/ }).parentElement!;
  fireEvent.dragStart(beta, { dataTransfer: data });
  const hobbi = screen.getByRole("button", { name: "Hobbi csoport összecsukása vagy kinyitása" }).closest("section")!;
  fireEvent.dragOver(hobbi, { dataTransfer: data });
  fireEvent.drop(hobbi, { dataTransfer: data });
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.path === "/sidebar/sections/s2/channels/c2")).toBe(true));
  await waitFor(() => expect(useToasts.getState().items.map((i) => i.text)).toContain("A szerver nem fogadta el a módosítást, visszaállt az előző sorrend."));
  expect(qc.getQueryData<Schemas["SidebarSection"][]>(conversationKeys.sections)?.find((s) => s.id === "s2")?.items).toEqual([]);
});

test("dragging a section onto another reorders the sections", async () => {
  const { calls } = setup();
  const data = { setData: () => {}, dropEffect: "", effectAllowed: "" };
  const header = screen.getByRole("button", { name: "Hobbi csoport összecsukása vagy kinyitása" }).parentElement!;
  fireEvent.dragStart(header, { dataTransfer: data });
  const munka = screen.getByRole("button", { name: "Munka csoport összecsukása vagy kinyitása" }).closest("section")!;
  fireEvent.dragOver(munka, { dataTransfer: data });
  fireEvent.drop(munka, { dataTransfer: data });
  await waitFor(() => expect(calls.find((c) => c.path === "/sidebar/sections/reorder")?.body).toEqual({ section_ids: ["s2", "s1"] }));
});

test("create, rename and delete a section", async () => {
  const { calls } = setup();
  fireEvent.click(screen.getByRole("button", { name: "Új szakasz" }));
  const create = await screen.findByRole("dialog", { name: "Új szakasz" });
  fireEvent.change(within(create).getByRole("textbox", { name: "Név" }), { target: { value: " Új " } });
  fireEvent.click(within(create).getByRole("button", { name: "Létrehozás" }));
  await waitFor(() => expect(calls.find((c) => c.method === "POST")?.body).toEqual({ name: "Új" }));

  openMenu("Munka szakasz műveletei");
  fireEvent.click(await screen.findByRole("menuitem", { name: "Átnevezés" }));
  const rename = await screen.findByRole("dialog", { name: "Szakasz átnevezése" });
  fireEvent.change(within(rename).getByRole("textbox", { name: "Név" }), { target: { value: "Meló" } });
  fireEvent.click(within(rename).getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(calls.find((c) => c.method === "PATCH")).toMatchObject({ path: "/sidebar/sections/s1", body: { name: "Meló" } }));

  // The name is "Meló" until the refetch brings the fixture's "Munka" back.
  const [first] = await screen.findAllByRole("button", { name: /szakasz műveletei$/ });
  fireEvent.pointerDown(first!, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(await screen.findByRole("menuitem", { name: "Szakasz törlése" }));
  const del = await screen.findByRole("dialog", { name: "Törlöd a szakaszt?" });
  expect(within(del).getByText(/visszakerülnek az alapértelmezett csoportjukba/)).toBeTruthy();
  fireEvent.click(within(del).getByRole("button", { name: "Szakasz törlése" }));
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.path === "/sidebar/sections/s1")).toBe(true));
});
