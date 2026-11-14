// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useParams } from "react-router";
import type { Schemas } from "@/shared/api";
import { identityKeys, type Actor } from "@/features/identity";
import { ProfileTrigger } from "./ProfilePopover";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "admin", state: "active", created_at: "" } as Actor;
const anna = { ...me, id: "h2", handle: "anna", display_name: "Anna", role: "member", status_text: "Szabadságon", description: "Tervező", timezone: "Asia/Tokyo" } as Actor;
const bot = { ...me, id: "a1", kind: "agent", handle: "trader-logs", display_name: "Trader", role: "member", effective_color: "teal" } as Actor;

function Where() {
  const loc = useLocation();
  return <p data-testid="where">{loc.pathname + loc.search}</p>;
}
function Opened() {
  return <p>opened {useParams().id}</p>;
}

function setup(actorId: string) {
  const posts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const path = new URL(req.url).pathname.replace("/api/v1", "");
      if (path === "/agents/a1/overview")
        return Response.json({ actor: bot, state: "active", assignment: { host_id: "h", host_name: "pc-dev", client: "claude", model: "opus" } } as unknown as Schemas["AgentOverview"]);
      if (path === "/dms") {
        posts.push(await req.json());
        return Response.json({ id: "dm9", kind: "dm", name: "dm", is_member: true, unread: 0, mentions: 0 });
      }
      return Response.json({}, { status: 404 });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([me, anna, bot].map((a) => [a.id, a])));
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "x" });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/c/x"]}>
        <Routes>
          <Route
            path="/c/x"
            element={
              <>
                <ProfileTrigger actorId={actorId}>name</ProfileTrigger>
                <Where />
              </>
            }
          />
          <Route path="/c/:id" element={<Opened />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return posts;
}

afterEach(() => vi.unstubAllGlobals());

test("a person's profile: status, description, local time, and DM opens the conversation", async () => {
  const posts = setup("h2");
  fireEvent.click(screen.getByRole("button", { name: "Anna profilja" }));
  const pop = await screen.findByRole("dialog", { name: "Anna profilja" });
  expect(pop.textContent).toContain("@anna");
  expect(pop.textContent).toContain("Szabadságon");
  expect(pop.textContent).toContain("Tervező");
  expect(pop.textContent).toMatch(/Helyi idő: \d\d:\d\d \(Asia\/Tokyo\)/);
  fireEvent.click(screen.getByRole("button", { name: "Üzenet" }));
  expect(await screen.findByText("opened dm9")).toBeTruthy();
  expect(posts).toEqual([{ actor_ids: ["h2"] }]);
});

test("an agent's profile shows state, machine and client; Áttekintés opens the overview pane", async () => {
  setup("a1");
  fireEvent.click(screen.getByRole("button", { name: "Trader profilja" }));
  expect(await screen.findByText("pc-dev")).toBeTruthy();
  expect(screen.getByText("claude · opus")).toBeTruthy();
  expect(screen.getByText("tétlen")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Áttekintés" }));
  await waitFor(() => expect(screen.getByTestId("where").textContent).toContain("work=agent"));
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("no DM button on one's own profile", async () => {
  setup("me");
  fireEvent.click(screen.getByRole("button", { name: "Norbi profilja" }));
  await screen.findByRole("dialog", { name: "Norbi profilja" });
  expect(screen.queryByRole("button", { name: "Üzenet" })).toBeNull();
});
