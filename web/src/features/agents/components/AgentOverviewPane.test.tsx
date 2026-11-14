// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import type { Schemas } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { useActivity } from "../activity";
import { AgentOverviewPane } from "./AgentOverviewPane";

const owner = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" } satisfies Schemas["Actor"];
const member = { ...owner, role: "member" } satisfies Schemas["Actor"];
const agent = { id: "ag", kind: "agent", handle: "backend-claude", display_name: "Backend", role: "member", state: "active", created_at: "", description: "Go backend", effective_color: "teal" } satisfies Schemas["Actor"];
const human = { id: "hu", kind: "human", handle: "anna", display_name: "Anna", role: "member", state: "active", created_at: "", timezone: "Europe/Budapest" } satisfies Schemas["Actor"];

const now = new Date();
const minutesAgo = (m: number) => new Date(now.getTime() - m * 60_000).toISOString();

const agentOverview: Schemas["AgentOverview"] = {
  actor: agent,
  state: "active",
  activity: { working: false },
  assignment: {
    host_id: "h1", host_name: "pc-dev", client: "claude", model: "opus", mode: "direct", sandbox: false, root_dir: "/home/n", worktree_repo: "",
    permission_mode: "auto", approvals: "chat", max_session_tokens: 100_000, daily_token_limit: 1_000_000, allow_web_read: true,
    grants: [{ id: "g1", path: "/srv/data", mode: "write", created_at: "" }],
  },
  threads: [{ channel_id: "c1", channel_name: "general", thread_root_id: "t1", session_id: "s1", last_run_tokens: 4000, context_tokens: 95_000, max_session_tokens: 100_000 }],
  usage: { today_tokens: 250_000, daily_token_limit: 1_000_000, last_runs: [] },
  collaboration: { notes: [], active_leases: [], open_questions: [] },
  inbox: { count: 3, oldest_at: minutesAgo(10) },
  common_channels: [{ id: "c1", name: "general", kind: "public" }],
};

function Location() {
  const loc = useLocation();
  return <output data-testid="loc">{loc.search}</output>;
}

function setup(opts: { me?: Schemas["Actor"]; overview?: Schemas["AgentOverview"]; id?: string; notFound?: boolean } = {}) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });
      calls.push({ method: req.method, path: u.pathname, body: req.method === "GET" ? undefined : await req.clone().text() });
      if (u.pathname.endsWith("/overview") && opts.notFound)
        return new Response(JSON.stringify({ type: "about:blank", title: "Not Found", status: 404, detail: "not found" }), { status: 404, headers: { "Content-Type": "application/problem+json" } });
      if (u.pathname.endsWith("/overview")) return json(opts.overview ?? agentOverview);
      if (u.pathname.endsWith("/hosts")) return json({ hosts: [{ id: "h1", name: "pc-dev", color: "blue", state: "approved", created_at: "", last_seen_at: now.toISOString(), report: {}, agents: [] }] });
      if (u.pathname.endsWith("/runs")) return json({ runs: [] });
      if (u.pathname.endsWith("/usage")) return json({ usage: [] });
      if (u.pathname.endsWith("/delegation-grants")) return json({ grants: [] });
      if (u.pathname.endsWith("/instructions")) return json({ text: "Be careful.\nWrite tests.", effective: "" });
      if (u.pathname.endsWith("/state")) return new Response(null, { status: 204 });
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.session, { actor: opts.me ?? owner, csrf_token: "" });
  const view = render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/c/c1?thread=t9"]}>
        <AgentOverviewPane agentId={opts.id ?? "ag"} terminals={<p>terminal list</p>} />
        <Location />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { calls, view };
}

beforeEach(() => localStorage.clear());
afterEach(() => {
  vi.unstubAllGlobals();
  useActivity.setState({ working: {}, steps: {} });
});

test("the header shows the state, machine, client and access chips, and the actions", async () => {
  useActivity.setState({ working: { ag: [{ channelId: "c1", threadRootId: "t1" }] }, steps: {} });
  setup();
  expect(await screen.findByText("Backend")).toBeTruthy();
  expect(screen.getByText("@backend-claude")).toBeTruthy();
  expect(screen.getByText("dolgozik")).toBeTruthy();
  expect(screen.getByText("pc-dev")).toBeTruthy();
  expect(screen.getByText("claude · opus")).toBeTruthy();
  expect(screen.getAllByText("Teljes hozzáférés").length).toBe(2); // the chip and the configuration row
  const actions = within(screen.getByRole("group", { name: "Műveletek" }));
  for (const name of ["DM", "Fájlok", "Terminálok", "Beállítások"]) expect(actions.getByRole("button", { name })).toBeTruthy();
  expect(screen.getByRole("button", { name: "További műveletek" })).toBeTruthy();
  // Sections: the near-full session, the configuration, the terminals slot.
  expect(screen.getByText("hamarosan új munkamenet")).toBeTruthy();
  expect(screen.getByText("Létrehozáskor rögzítve")).toBeTruthy();
  expect(screen.getByText("terminal list")).toBeTruthy();
  expect(await screen.findByText(/Be careful\./)).toBeTruthy();
});

test("the inbox line shows the count and the oldest item's age", async () => {
  setup();
  expect(await screen.findByText(/Feldolgozatlan a postaládában: 3 · a legrégebbi 10 perccel ezelőtt/)).toBeTruthy();
});

test("a collapsed section stays collapsed for this viewer", async () => {
  const { view } = setup();
  const toggle = await screen.findByRole("button", { name: "Most" });
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByText(/Feldolgozatlan/)).toBeNull();
  view.unmount();
  setup();
  expect((await screen.findByRole("button", { name: "Most" })).getAttribute("aria-expanded")).toBe("false");
  expect(screen.getByRole("button", { name: "Munkamenetek" }).getAttribute("aria-expanded")).toBe("true");
});

test("settings and details links only set the URL params, keeping the others", async () => {
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "Beállítások" }));
  let q = new URLSearchParams(screen.getByTestId("loc").textContent ?? "");
  expect([q.get("agent"), q.get("settings"), q.get("thread")]).toEqual(["backend-claude", "profil", "t9"]);
  fireEvent.click(screen.getByRole("button", { name: "Részletek" }));
  q = new URLSearchParams(screen.getByTestId("loc").textContent ?? "");
  expect(q.get("settings")).toBe("konfiguracio");
  fireEvent.click(await screen.findByRole("button", { name: "Megtekintés" }));
  q = new URLSearchParams(screen.getByTestId("loc").textContent ?? "");
  expect(q.get("settings")).toBe("utasitas");
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("an admin stops the agent after confirming", async () => {
  const { calls } = setup();
  const more = await screen.findByRole("button", { name: "További műveletek" });
  fireEvent.pointerDown(more, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(await screen.findByRole("menuitem", { name: "Leállítás" }));
  const dialog = await screen.findByRole("dialog", { name: "Leállítod @backend-claude agentet?" });
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Leállítás" })));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.path.endsWith("/actors/ag/state") && c.body === JSON.stringify({ state: "stopped" }))).toBe(true));
});

test("members do not get the admin menu", async () => {
  setup({ me: member });
  expect(await screen.findByRole("button", { name: "Beállítások" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "További műveletek" })).toBeNull();
});

test("a human gets the small version: profile and common channels", async () => {
  setup({ id: "hu", overview: { actor: human, state: "active", common_channels: [{ id: "c2", name: "random", kind: "public" }] } });
  expect(await screen.findByText("Anna")).toBeTruthy();
  expect(screen.getByText("#random")).toBeTruthy();
  expect(screen.getByRole("button", { name: "DM" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Munkamenetek" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Beállítások" })).toBeNull();
});

test("another person's agent: a calm note, no retries and no polling (round 38)", async () => {
  const { calls } = setup({ me: member, notFound: true });
  expect(await screen.findByText("Ez az agent mást illet")).toBeTruthy();
  expect(screen.getByText(/csak a tulajdonosa és az adminok látják/)).toBeTruthy();
  expect(calls.filter((c) => c.path.endsWith("/overview")).length).toBe(1);
});
