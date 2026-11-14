// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import TasksPage from "./TasksPage";

const task = (id: string, title: string, status: string, rank: string) => ({ id, title, description: "", status, position: Number(rank.charCodeAt(0)), status_changed_at: "2026-10-15T08:00:00Z", created_at: "2026-10-15T08:00:00Z", updated_at: "2026-10-15T08:00:00Z" });
const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

function setup() {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const path = new URL(req.url).pathname;
      const body = req.method === "GET" || req.method === "DELETE" ? null : await req.json();
      calls.push({ method: req.method, path, body });
      if (path.endsWith("/planner/tasks") && req.method === "GET")
        return json({ tasks: [task("a", "Első", "started", "a"), task("b", "Második", "started", "b"), task("c", "Kész munka", "done", "c"), task("d", "Később", "scheduled", "d")] });
      return json(task("x", "Új", "scheduled", "z"));
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <TasksPage />
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

test("groups by status, done collapsed; quick add posts the title", async () => {
  const calls = setup();
  expect(await screen.findByText("Első")).toBeTruthy();
  expect(screen.getByText("Később")).toBeTruthy();
  expect(screen.queryByText("Kész munka")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /Befejezve \(14 nap\)/ }));
  expect(screen.getByText("Kész munka")).toBeTruthy();
  const input = screen.getByRole("textbox", { name: "Új feladat…" });
  fireEvent.change(input, { target: { value: "  Írj tesztet " } });
  fireEvent.submit(input);
  await waitFor(() => expect(calls.find((c) => c.method === "POST")?.body).toEqual({ title: "Írj tesztet" }));
});

test("the status circle cycles, the arrows move between neighbours", async () => {
  const calls = setup();
  const row = (await screen.findByText("Második")).closest("li")!;
  fireEvent.click(within(row).getByRole("button", { name: "Feljebb" }));
  await waitFor(() => expect(calls.find((c) => c.method === "PATCH")?.body).toEqual({ move: true, before_id: "a" }));
  fireEvent.click(within(screen.getByText("Később").closest("li")!).getByRole("button", { name: /Állapot léptetése/ }));
  await waitFor(() => expect(calls.filter((c) => c.method === "PATCH")[1]?.body).toEqual({ status: "started" }));
});

test("the board view is remembered", async () => {
  setup();
  fireEvent.click(await screen.findByRole("radio", { name: "Tábla" }));
  expect(screen.getByRole("region", { name: "Elkezdve" })).toBeTruthy();
  expect(localStorage.getItem("colony.tasks.view")).toContain("board");
});
