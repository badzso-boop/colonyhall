// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useParams } from "react-router";
import { identityKeys, type Actor } from "@/features/identity";
import { NewConversationDialog, candidates } from "./NewConversationDialog";

const actor = (id: string, kind: string, handle: string, extra = {}) => ({ id, kind, handle, display_name: handle, role: "member", state: "active", created_at: "", ...extra }) as Actor;
const people = [actor("me", "human", "norbi"), actor("h2", "human", "anna"), actor("a1", "agent", "trader-logs"), actor("a2", "agent", "personal-agy"), actor("a3", "agent", "old", { archived_at: "2026-10-30T00:00:00Z" })];

test("candidates: agents for an AI conversation, people for a DM, never yourself or archived agents", () => {
  expect(candidates(people, "agentDm", "me", "").map((a) => a.handle)).toEqual(["personal-agy", "trader-logs"]);
  expect(candidates(people, "dm", "me", "").map((a) => a.handle)).toEqual(["anna"]);
  expect(candidates(people, "agentDm", "me", "trad").map((a) => a.handle)).toEqual(["trader-logs"]);
});

function Opened() {
  return <p>opened {useParams().id}</p>;
}

test("choosing an agent opens the DM in the new UI", async () => {
  const posts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      posts.push(await req.json());
      return new Response(JSON.stringify({ id: "dm1", kind: "dm", name: "dm-x", is_member: true, unread: 0, mentions: 0 }), { headers: { "Content-Type": "application/json" } });
    }),
  );
  const qc = new QueryClient();
  qc.setQueryData(identityKeys.actors, new Map(people.map((a) => [a.id, a])));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/"]}>
        <Routes>
          <Route path="/" element={<NewConversationDialog kind="agentDm" meId="me" onClose={() => {}} />} />
          <Route path="/c/:id" element={<Opened />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(screen.getByRole("button", { name: /trader-logs/ }));
  expect(await screen.findByText("opened dm1")).toBeTruthy();
  await waitFor(() => expect(posts).toEqual([{ actor_ids: ["a1"] }]));
  vi.unstubAllGlobals();
});
