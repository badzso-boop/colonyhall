// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import RunsPage from "./RunsPage";

type Actor = Schemas["Actor"];
const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });
const actor = (id: string, kind: Actor["kind"], role: Actor["role"], extra: Partial<Actor> = {}): Actor => ({ id, kind, handle: id, display_name: id, role, state: "active", created_at: "", ...extra });

afterEach(() => vi.unstubAllGlobals());

// Round 38: the "Személy" filter narrows the usage and the run list to one person's agents.
test("the person filter asks the server for one person's runs and usage", async () => {
  const urls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      urls.push(u.pathname + u.search);
      const p = u.pathname.replace(/^\/api\/v1/, "");
      if (p.endsWith("/auth/session")) return json({ actor: actor("norbi", "human", "owner"), csrf_token: "x" });
      if (p === "/usage") return json({ usage: [], groups: [] });
      if (p === "/runs") return json({ runs: [] });
      if (p === "/hosts") return json({ hosts: [] });
      if (p === "/channels") return json({ channels: [] });
      if (p === "/settings/currency") return json({});
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(
    identityKeys.actors,
    new Map([
      ["norbi", actor("norbi", "human", "owner")],
      ["kollega", actor("kollega", "human", "member", { display_name: "Kolléga" })],
      ["coder", actor("coder", "agent", "member", { owner_id: "kollega" })],
      ["ops", actor("ops", "agent", "member")],
    ]),
  );
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <MemoryRouter>
          <RunsPage />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  const person = await screen.findByRole("combobox", { name: "Személy" });
  fireEvent.change(person, { target: { value: "kollega" } });
  await waitFor(() => expect(urls.some((x) => x.startsWith("/api/v1/usage") && x.includes("owner_id=kollega"))).toBe(true));
  await waitFor(() => expect(urls.some((x) => x.startsWith("/api/v1/runs") && x.includes("owner_id=kollega"))).toBe(true));
  // The agent filter offers only that person's agents.
  const agentSelect = screen.getByRole("combobox", { name: /agent/i });
  expect([...agentSelect.querySelectorAll("option")].map((o) => o.textContent)).toEqual(["Minden agent", "@coder"]);
});
