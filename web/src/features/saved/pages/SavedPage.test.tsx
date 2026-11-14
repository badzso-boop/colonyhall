// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import SavedPage from "./SavedPage";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "", timezone: "Europe/Budapest" };
const items = [
  {
    id: "s1", message_id: "m1", channel_id: "c1", note: "", created_at: "2026-11-14T10:00:00Z", available: true,
    message: { id: "m1", channel_id: "c1", channel_name: "general", channel_kind: "public", author_id: "me", body: "a számla kérdése", created_at: "2026-11-14T09:00:00Z" },
  },
  { id: "s2", message_id: "m2", channel_id: "c9", note: "nézd meg", created_at: "2026-11-14T09:00:00Z", available: false, remind_at: "2026-11-15T08:00:00Z" },
];

function setup() {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      calls.push({ method: req.method, path: u.pathname + u.search, body: req.method === "GET" ? undefined : await req.clone().json().catch(() => null) });
      if (req.method === "GET") return new Response(JSON.stringify({ items }), { headers: { "Content-Type": "application/json" } });
      return new Response(JSON.stringify(items[0]), { headers: { "Content-Type": "application/json" } });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "" });
  qc.setQueryData(identityKeys.actors, new Map([[me.id, me]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TooltipProvider>
          <SavedPage />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

test("lists saved messages, jumps to them, hides a message no longer readable, and marks done", async () => {
  const calls = setup();
  const link = await screen.findByRole("link", { name: /a számla kérdése/ });
  expect(link.getAttribute("href")).toBe("/c/c1?msg=m1");
  expect(screen.getByText("nézd meg")).toBeTruthy();
  expect(screen.getByText("Ez az üzenet már nem érhető el neked.")).toBeTruthy();
  fireEvent.click(screen.getAllByRole("button", { name: "Kész" })[0]!);
  await waitFor(() => expect(calls.some((c) => c.method === "PATCH" && c.path === "/api/v1/me/saved/s1" && (c.body as { done?: boolean }).done === true)).toBe(true));
});

test("the filters ask the server for reminders and done items", async () => {
  const calls = setup();
  fireEvent.click(await screen.findByRole("radio", { name: "Emlékeztetővel" }));
  await waitFor(() => expect(calls.some((c) => c.path === "/api/v1/me/saved?filter=reminders")).toBe(true));
});
