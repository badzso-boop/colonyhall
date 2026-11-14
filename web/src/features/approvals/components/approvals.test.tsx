// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import type { Schemas } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { ApprovalCard } from "./ApprovalCard";
import { DelegationActions } from "./DelegationActions";
import { GrantBar } from "./GrantBar";

const norbi = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" } satisfies Schemas["Actor"];
const inAnHour = () => new Date(Date.now() + 3_600_000).toISOString();

const approval = (over: Partial<Schemas["ApprovalRequest"]> = {}): Schemas["ApprovalRequest"] => ({
  id: "ap1", agent_id: "ag", channel_id: "c1", tool_name: "Bash", input: '{"command":"rm -rf build","token":"***"}', state: "pending",
  created_at: new Date().toISOString(), expires_at: new Date(Date.now() + 125_000).toISOString(), ...over,
});

type Handler = (req: Request, u: URL) => Promise<Response | undefined> | Response | undefined;
const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });

function setup(ui: ReactNode, handler: Handler) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      calls.push({ method: req.method, path: u.pathname, body: req.method === "GET" ? undefined : await req.clone().json().catch(() => null) });
      return (await handler(req, u)) ?? json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([[norbi.id, norbi]]));
  render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
  return { calls };
}

afterEach(() => vi.unstubAllGlobals());

test("a pending approval: summary, countdown, masked details, and Engedélyezem decides it in place", async () => {
  const { calls } = setup(<ApprovalCard id="ap1" />, (req) =>
    req.method === "POST" ? json(approval({ state: "allowed", decided_by: "me", decided_at: new Date().toISOString() })) : json(approval()),
  );
  const card = await screen.findByRole("region", { name: "Engedélykérés: Bash" });
  expect(within(card).getByText("rm -rf build")).toBeTruthy();
  expect(within(card).getByText(/automatikus elutasítás: 2:0\d/)).toBeTruthy();
  fireEvent.click(within(card).getByRole("button", { name: "Részletek" }));
  const sheet = await screen.findByRole("dialog");
  expect(sheet.textContent).toContain('"token": "***"');
  fireEvent.keyDown(sheet, { key: "Escape" });

  fireEvent.click(within(card).getByRole("button", { name: "Engedélyezem" }));
  await waitFor(() => expect(calls.find((c) => c.method === "POST")).toEqual({ method: "POST", path: "/api/v1/approvals/ap1", body: { decision: "allow" } }));
  await waitFor(() => expect(within(card).getByText(/@norbi engedélyezte/)).toBeTruthy());
  expect(within(card).queryByRole("button", { name: "Elutasítom" })).toBeNull();
  expect(within(card).getByText("engedélyezve")).toBeTruthy();
});

const delegation = (over: Partial<Schemas["Delegation"]> = {}): Schemas["Delegation"] => ({
  id: "d1", workspace_id: "w", message_id: "m1", channel_id: "c1", from_agent_id: "a", to_agent_id: "b", to_agent_handle: "builder",
  question_id: "q7", state: "pending", created_at: "", ...over,
});

test("delegation: admins approve by answering its question; others see the waiting chip", async () => {
  const { calls } = setup(<DelegationActions message={{ id: "m1" }} admin />, (_r, u) => (u.pathname.endsWith("/delegations") ? json({ delegations: [delegation()] }) : undefined));
  fireEvent.click(await screen.findByRole("button", { name: "Jóváhagyom" }));
  await waitFor(() => expect(calls.find((c) => c.method === "POST")).toEqual({ method: "POST", path: "/api/v1/questions/q7/answer", body: { answer: "Jóváhagyom" } }));
});

test("delegation states after the decision", async () => {
  setup(
    <>
      <DelegationActions message={{ id: "m1" }} admin={false} />
      <DelegationActions message={{ id: "m2" }} admin />
      <DelegationActions message={{ id: "m3" }} admin />
    </>,
    () =>
      json({
        delegations: [
          delegation(),
          delegation({ id: "d2", message_id: "m2", state: "denied", decided_by_handle: "norbi", decided_at: "2026-11-01T10:00:00Z" }),
          delegation({ id: "d3", message_id: "m3", state: "approved", approved_by_grant: true }),
        ],
      }),
  );
  expect(await screen.findByText("jóváhagyásra vár")).toBeTruthy();
  expect(screen.getByText(/elutasítva · @norbi/)).toBeTruthy();
  expect(screen.getByText("✅ jóváhagyva az engedély alapján")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Jóváhagyom" })).toBeNull();
});

const grant = (over: Partial<Schemas["DelegationGrant"]> = {}): Schemas["DelegationGrant"] => ({
  id: "g1", workspace_id: "w", channel_id: "c1", agent_a_id: "a", agent_a_handle: "planner", agent_b_id: "b", agent_b_handle: "builder",
  delegation_id: "d1", approved_by: "me", approver_handle: "norbi", approved_at: "", last_used_at: "", expires_at: inAnHour(), effective_expires_at: inAnHour(), ...over,
});

test("the grant bar: the DM's grant, revoked by an admin after an in-page confirmation", async () => {
  const { calls } = setup(<GrantBar channelId="c1" isDm admin />, (req, u) =>
    req.method === "POST" ? json(grant({ revoked_at: new Date().toISOString() })) : u.pathname.endsWith("/delegation-grants") ? json({ grants: [grant(), grant({ id: "g2", thread_root_id: "t1" })] }) : undefined,
  );
  const bar = await screen.findByRole("status");
  expect(bar.textContent).toMatch(/🤝 @planner ↔ @builder: engedély, lejár \d\d:\d\d/);
  expect(within(bar).getAllByRole("listitem")).toHaveLength(1);
  fireEvent.click(within(bar).getByRole("button", { name: "Visszavonás" }));
  fireEvent.click(within(bar).getByRole("button", { name: "Igen, visszavonom" }));
  await waitFor(() => expect(calls.find((c) => c.method === "POST")?.path).toBe("/api/v1/delegation-grants/g1/revoke"));
});

test("the grant bar has no revoke for non-admins and nothing in a channel's main stream", async () => {
  setup(
    <>
      <GrantBar channelId="c1" isDm admin={false} />
      <GrantBar channelId="c1" isDm={false} admin />
    </>,
    () => json({ grants: [grant()] }),
  );
  expect(await screen.findByRole("status")).toBeTruthy();
  expect(screen.getAllByRole("status")).toHaveLength(1);
  expect(screen.queryByRole("button", { name: "Visszavonás" })).toBeNull();
});
