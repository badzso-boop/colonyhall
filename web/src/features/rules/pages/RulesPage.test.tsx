// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import RulesPage from "./RulesPage";

const actor = (id: string, kind: string, handle: string, role = "member") => ({ id, kind, handle, display_name: handle, role, state: "active", created_at: "" });
const me = actor("h", "human", "norbi", "owner");
const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });
const v2 = "Te vagy @{handle}.\n\n**Kérdezd** {admins} adminokat.";

function setup() {
  const puts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      if (u.pathname.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      if (u.pathname.endsWith("/agent-rules/1")) return json({ version: 1, text: "Régi szöveg." });
      if (req.method === "PUT") {
        puts.push(await req.json());
        return json({ version: 3, text: "x" });
      }
      return json({ version: 2, text: v2, created_by: "h", created_at: "2026-10-29T10:00:00Z", history: [{ version: 1, created_by: "h" }, { version: 2, created_by: "h" }] });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["h", me], ["a", actor("a", "agent", "backend")]]));
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <RulesPage />
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return puts;
}

afterEach(() => vi.unstubAllGlobals());

test("reading shows placeholders as chips; previewing as an agent fills them", async () => {
  setup();
  expect(await screen.findByText("{handle}")).toBeTruthy();
  expect(screen.getByText("{admins}")).toBeTruthy();
  fireEvent.change(screen.getByRole("combobox", { name: "Előnézet @agentként" }), { target: { value: "a" } });
  expect(await screen.findByText("Te vagy @backend.")).toBeTruthy();
  expect(screen.getByText(/@norbi adminokat/)).toBeTruthy();
});

test("editing: insert a placeholder, count, save with a diff", async () => {
  const puts = setup();
  fireEvent.click(await screen.findByRole("button", { name: "Szerkesztés" }));
  const area = screen.getByRole("textbox", { name: "Szöveg" }) as HTMLTextAreaElement;
  fireEvent.change(area, { target: { value: `${v2}\n\nMunkaterület: ` } });
  area.setSelectionRange(area.value.length, area.value.length);
  fireEvent.click(screen.getByRole("button", { name: "{workspace}" }));
  expect(area.value.endsWith("Munkaterület: {workspace}")).toBe(true);
  expect(screen.getByText(`${[...area.value].length} / 8000 karakter`)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Mentés új verzióként" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/\+ Munkaterület: \{workspace\}/)).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Mentés új verzióként" }));
  await waitFor(() => expect(puts).toEqual([{ text: `${v2}\n\nMunkaterület: {workspace}` }]));
});

test("comparing an older version with the current one", async () => {
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "Összevetés ezzel: 1. verzió" }));
  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByText(/− Régi szöveg\./)).toBeTruthy();
});
