// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useSearchParams } from "react-router";
import { identityKeys } from "@/features/identity";
import { NotesWindow } from "../components/NotesWindow";
import NotesPage from "./NotesPage";

const actor = (id: string, kind: string, handle: string) => ({ id, kind, handle, display_name: handle, role: "member", state: "active", created_at: "" });
const msg = (id: string, author: string, body: string) => ({ id, channel_id: "c", author_id: author, kind: "text", body, mentions: [], reply_count: 0, created_at: "2026-10-30T08:00:00Z" });
const note = (id: string, agent: string, thread: string, at: string, text: string) => ({ id, agent_id: agent, thread_root_id: thread, text, created_at: at });
const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });

const notes: Record<string, ReturnType<typeof note>[]> = {
  a1: [note("n1", "a1", "t1", "2026-10-30T09:00:00Z", "Első **jegyzet**"), note("n2", "a1", "t2", "2026-10-30T10:00:00Z", "Második szál")],
  a2: [note("n3", "a2", "t1", "2026-10-30T11:00:00Z", "A frontend jegyzete")],
};

function setup(ui: React.ReactNode, url = "/") {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      asked.push(u.pathname + u.search);
      const m = u.pathname.match(/agents\/(\w+)\/notes/);
      if (m) {
        const thread = u.searchParams.get("thread");
        return json({ notes: (notes[m[1]!] ?? []).filter((n) => !thread || n.thread_root_id === thread) });
      }
      const t = u.pathname.match(/messages\/(\w+)\/thread/);
      if (t) return json({ root: msg(t[1]!, "h", `Szál ${t[1]}`), replies: [msg("r1", "a1", "ok"), msg("r2", "a2", "ok")] });
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["h", actor("h", "human", "norbi")], ["a1", actor("a1", "agent", "backend")], ["a2", actor("a2", "agent", "frontend")]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[url]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
  return asked;
}

function ThreadParam() {
  const [p] = useSearchParams();
  return <output aria-label="thread">{p.get("thread")}</output>;
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

test("the page lists notes by agent and thread, shows the newest thread and searches", async () => {
  setup(
    <>
      <NotesPage />
      <ThreadParam />
    </>,
  );
  const backend = await screen.findByRole("region", { name: "@backend" });
  expect(await within(backend).findByText("Szál t2")).toBeTruthy();
  expect(await screen.findByText("Második szál")).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "Keresés a jegyzetekben…" }), { target: { value: "frontend" } });
  await waitFor(() => expect(screen.queryByRole("region", { name: "@backend" })).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: /Szál t1/ }));
  expect(await screen.findByText("A frontend jegyzete")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Szál megnyitása" }));
  expect(screen.getByRole("status", { name: "thread" }).textContent).toBe("t1");
});

test("the window follows the open thread, asks each agent author, and can be pinned", async () => {
  const asked = setup(<NotesWindow />, "/c/x?thread=t1");
  expect(await screen.findByText("A frontend jegyzete")).toBeTruthy();
  expect(screen.getByText("jegyzet")).toBeTruthy();
  expect(asked).toContain("/api/v1/agents/a1/notes?thread=t1");
  expect(asked).toContain("/api/v1/agents/a2/notes?thread=t1");
  fireEvent.click(screen.getByRole("button", { name: "Rögzítés ehhez a szálhoz" }));
  expect(localStorage.getItem("colony.notes.pin")).toContain("t1");
});

test("with no thread open, the window asks for one", () => {
  setup(<NotesWindow />);
  expect(screen.getByText("Nyiss meg egy szálat")).toBeTruthy();
});
