// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { identityKeys } from "@/features/identity";
import { FileBrowserPane } from "./FileBrowserPane";

const agent = { id: "ag", kind: "agent", handle: "backend", display_name: "Backend", role: "member", state: "active", created_at: "" };
const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

function setup(fileFetch: (path: string) => unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const path = new URL(req.url).pathname;
      if (path.endsWith("/overview")) return json({ actor: agent, state: "active", common_channels: [{ id: "c1", name: "general", kind: "public" }] });
      if (path.endsWith("/fs")) {
        const b = await req.json();
        if (b.op === "git_status") return json({ git_status: [{ root: "/r", branch: "main", ahead: 1, behind: 0, files: [{ path: "src/app.go", status: " M" }] }] });
        if (b.op === "list" && b.path === "") return json({ entries: [{ name: "README.md", kind: "file" }, { name: "src", kind: "dir" }, { name: "node_modules", kind: "dir", heavy: true }] });
        if (b.op === "list" && b.path === "src") return json({ entries: [{ name: "app.go", kind: "file" }] });
        return json({ entries: [] });
      }
      if (path.endsWith("/file-fetch")) return json(fileFetch((await req.json()).path));
      if (path.includes("/files/f1/content")) return new Response("# Cím\n\n**vastag** <b>nem html</b>");
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([[agent.id, agent]]));
  render(
    <QueryClientProvider client={qc}>
      <FileBrowserPane agentId="ag" path="" />
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

test("the tree: folders first, git letters, a dot on changed folders, the branch", async () => {
  setup(() => ({}));
  const items = await screen.findAllByRole("treeitem");
  expect(items.map((i) => i.textContent)).toEqual(["node_modules", "src", "README.md"]);
  expect(screen.getByText("· ág: main")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /^src/ }));
  expect(await screen.findByText("app.go")).toBeTruthy();
  expect(screen.getByText("M")).toBeTruthy();
});

test("opening a markdown file fetches it and renders it; raw shows the text", async () => {
  setup((path) => ({ id: "q1", state: "done", path, file: { id: "f1", name: "README.md", mime: "text/markdown", size: 40, fetched_at: "" } }));
  fireEvent.click(await screen.findByRole("button", { name: /README.md/ }));
  expect(await screen.findByRole("heading", { name: "Cím" })).toBeTruthy();
  expect(document.querySelector("b")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Nyers" }));
  await waitFor(() => expect(screen.queryByRole("heading", { name: "Cím" })).toBeNull());
});

test("a refused file shows why", async () => {
  setup((path) => ({ id: "q2", state: "failed", path, error_code: "denied" }));
  fireEvent.click(await screen.findByRole("button", { name: /README.md/ }));
  expect(await screen.findByText("Ez a fájl vagy mappa nem nyitható meg.")).toBeTruthy();
});
