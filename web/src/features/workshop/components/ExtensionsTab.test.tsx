// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Schemas } from "@/shared/api";
import { ExtensionsTab } from "./ExtensionsTab";

const json = (b: unknown, status = 200) =>
  new Response(JSON.stringify(b), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const draft: Schemas["Extension"] = {
  id: "e1",
  slug: "hello",
  title: "Hello",
  description: "",
  bot_id: "b1",
  bot_handle: "hello-bot",
  maintainer_handle: "norbi",
  state: "draft",
  approval_off: false,
  created_at: "2026-11-17T09:00:00Z",
  updated_at: "2026-11-17T09:00:00Z",
  versions: [
    {
      version: 1,
      approved: false,
      created_at: "2026-11-17T09:00:00Z",
      created_by: "u1",
      tools: [{ name: "greet", kind: "js", risk: "read" }],
    },
  ],
};

const v1: Schemas["ExtensionVersion"] = {
  version: 1,
  manifest: {
    tools: [{ name: "greet", kind: "js", risk: "read" }],
    egress: ["api.example.com"],
  },
  source: "export const greet = () => 1",
  approved: false,
  created_at: "2026-11-17T09:00:00Z",
  created_by: "u1",
  diff: {
    code_changed: true,
    new_tools: ["greet"],
    new_egress: ["api.example.com"],
  },
};

function show(extensions: Schemas["Extension"][], runner = true) {
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (req.method !== "GET") {
        const body = await req.json().catch(() => null);
        calls.push({ method: req.method, path, body });
        if (path.endsWith("/test"))
          return json({ output: "Hello Teszt", is_error: false, logs: ["hi"] });
        if (path.endsWith("/workshop/extensions")) return json(draft, 201);
        return json({ ...draft, state: "live", live_version: 1 });
      }
      if (path.endsWith("/workshop/extensions"))
        return json({ extensions, runner_available: runner });
      if (path.endsWith("/versions/1")) return json(v1);
      return json({});
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ExtensionsTab owner />
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

test("a new extension takes a short name from its title and its bot's name", async () => {
  const calls = show([]);
  fireEvent.click(await screen.findByRole("button", { name: "Új bővítmény" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Név" }), {
    target: { value: "Napi Jira" },
  });
  fireEvent.change(screen.getByRole("textbox", { name: /^A bot neve/ }), {
    target: { value: "jira-bot" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Létrehozás" }));
  await waitFor(() =>
    expect(calls).toContainEqual({
      method: "POST",
      path: "/api/v1/workshop/extensions",
      body: { slug: "napi_jira", title: "Napi Jira", bot_handle: "jira-bot" },
    }),
  );
});

test("a version shows what it adds before it is approved, and can be tested", async () => {
  const calls = show([draft]);
  expect(await screen.findByText("Bot: @hello-bot")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Megnyitás" }));
  expect(
    await screen.findByText("Új kifelé engedett hosztok: api.example.com"),
  ).toBeTruthy();
  expect(screen.getByText("Új eszközök: greet")).toBeTruthy();
  fireEvent.change(screen.getByRole("textbox", { name: "Bemenet (JSON)" }), {
    target: { value: '{"name":"Teszt"}' },
  });
  fireEvent.click(screen.getByRole("button", { name: "Futtatás" }));
  expect(await screen.findByText("Hello Teszt")).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: "Jóváhagyás és élesítés" }),
  );
  await waitFor(() =>
    expect(calls.map((c) => c.path)).toContain(
      "/api/v1/workshop/extensions/e1/versions/1/approve",
    ),
  );
  expect(calls).toContainEqual({
    method: "POST",
    path: "/api/v1/workshop/extensions/e1/versions/1/test",
    body: { tool: "greet", input: { name: "Teszt" } },
  });
});

test("without a runner the tab says js tools do not run", async () => {
  show([], false);
  expect(await screen.findByText(/COLONY_EXT_RUNNER_URL/)).toBeTruthy();
});
