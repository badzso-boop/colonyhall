// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { identityKeys } from "@/features/identity";
import ActivityPage from "./ActivityPage";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const agent = { id: "ag", kind: "agent", handle: "backend", display_name: "Backend", role: "member", state: "active", created_at: "" };
const items = [
  { id: "a1", kind: "mention", created_at: "2026-11-14T10:00:00Z", read: false, available: true, channel_id: "c1", channel_name: "general", channel_kind: "public", message_id: "m1", actor_id: "ag", excerpt: "@norbi nézd meg" },
  { id: "a2", kind: "reaction", created_at: "2026-11-14T09:00:00Z", read: true, available: false, emoji: "👍", actor_id: "ag" },
];

function Where() {
  const l = useLocation();
  return <p data-testid="where">{l.pathname + l.search}</p>;
}

test("lists what concerns me; opening an item marks it read and jumps to the message", async () => {
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      calls.push({ method: req.method, path: u.pathname + u.search, body: req.method === "GET" ? null : await req.clone().json().catch(() => null) });
      if (req.method === "GET") return new Response(JSON.stringify({ items, unread_count: 1 }), { headers: { "Content-Type": "application/json" } });
      return new Response(JSON.stringify({ marked: 1 }), { headers: { "Content-Type": "application/json" } });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "" });
  qc.setQueryData(identityKeys.actors, new Map([[me.id, me], [agent.id, agent]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/activity"]}>
        <Routes>
          <Route path="*" element={<><ActivityPage /><Where /></>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("@backend megemlített")).toBeTruthy();
  expect(screen.getByText("Ehhez már nincs hozzáférésed.")).toBeTruthy();
  fireEvent.click(screen.getByText("@backend megemlített"));
  await waitFor(() => expect(calls.some((c) => c.method === "POST" && (c.body as { item_ids?: string[] })?.item_ids?.[0] === "a1")).toBe(true));
  expect(screen.getByTestId("where").textContent).toBe("/c/c1?msg=m1");
  vi.unstubAllGlobals();
});
