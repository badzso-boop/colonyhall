// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { BuilderTab } from "./BuilderTab";

const json = (b: unknown) =>
  new Response(JSON.stringify(b), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
const actor = (
  id: string,
  kind: string,
  role: string,
  handle: string,
  owner?: string,
) => ({
  id,
  kind,
  role,
  handle,
  display_name: handle,
  state: "active",
  created_at: "2026-11-17T09:00:00Z",
  ...(owner ? { owner_id: owner } : {}),
});

test("only agents of an owner or an admin can be made Builders", async () => {
  const calls: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (req.method === "POST") {
        calls.push(await req.json());
        return json({
          id: "g1",
          agent_id: "a1",
          target_kind: "workshop",
          granted_by: "o",
          created_at: "2026-11-17T09:00:00Z",
        });
      }
      if (path.endsWith("/actors"))
        return json({
          actors: [
            actor("o", "human", "owner", "norbi"),
            actor("m", "human", "member", "bela"),
            actor("a1", "agent", "member", "epito", "o"),
            actor("a2", "agent", "member", "masik", "m"),
          ],
        });
      if (path.endsWith("/workshop/grants")) return json({ grants: [] });
      return json({});
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <BuilderTab />
    </QueryClientProvider>,
  );
  const select = await screen.findByRole("combobox", { name: /^Agent/ });
  expect(screen.queryByRole("option", { name: /masik/ })).toBeNull();
  fireEvent.change(select, { target: { value: "a1" } });
  fireEvent.click(screen.getByRole("button", { name: "Építővé teszem" }));
  await waitFor(() =>
    expect(calls).toContainEqual({ agent_id: "a1", target_kind: "workshop" }),
  );
  vi.unstubAllGlobals();
});
