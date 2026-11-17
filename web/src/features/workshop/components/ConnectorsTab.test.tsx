// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Schemas } from "@/shared/api";
import { ConnectorsTab } from "./ConnectorsTab";

const json = (b: unknown, status = 200) =>
  new Response(JSON.stringify(b), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const jira: Schemas["Connector"] = {
  id: "c1",
  slug: "jira",
  title: "Jira",
  kind: "mcp",
  base_url: "https://mcp.example.com/mcp",
  auth: "bearer",
  has_secret: true,
  secret_hint: "abcd",
  approval_off: false,
  state: "ok",
  created_at: "2026-11-17T09:00:00Z",
  updated_at: "2026-11-17T09:00:00Z",
  tools: [
    {
      name: "search",
      description: "Search issues",
      risk: "read",
      effective_risk: "read",
    },
    {
      name: "create_issue",
      description: "Create",
      risk: "write",
      effective_risk: "write",
    },
  ],
};

function show(
  owner: boolean,
  connectors: Schemas["Connector"][] = [jira],
  keys = true,
) {
  const calls: Array<{ method: string; path: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (req.method !== "GET") {
        const body = req.headers.get("Content-Type")?.includes("json")
          ? await req.json().catch(() => null)
          : null;
        calls.push({ method: req.method, path, body });
        return json(
          jira,
          req.method === "POST" && path.endsWith("/connectors") ? 201 : 200,
        );
      }
      if (path.endsWith("/workshop/connectors"))
        return json({ connectors, secrets_available: keys });
      return json({});
    }),
  );
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <ConnectorsTab owner={owner} />
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

test("a connector shows its secret only by its hint, and only the owner can switch the approval off", async () => {
  show(false);
  expect(await screen.findByText("Jira")).toBeTruthy();
  expect(screen.getByText(/…abcd/)).toBeTruthy();
  const sw = screen.getByRole("switch", {
    name: "Író és kifelé küldő hívások jóváhagyás nélkül",
  });
  expect(sw.hasAttribute("disabled")).toBe(true);
});

test("the owner switches the approval off", async () => {
  const calls = show(true);
  const sw = await screen.findByRole("switch", {
    name: "Író és kifelé küldő hívások jóváhagyás nélkül",
  });
  fireEvent.click(sw);
  await waitFor(() =>
    expect(calls).toContainEqual({
      method: "PUT",
      path: "/api/v1/workshop/connectors/c1/approval",
      body: { approval_off: true },
    }),
  );
});

test("a new connector gets a short name from its title, needs an address and a secret, and sends the secret once", async () => {
  const calls = show(true, []);
  fireEvent.click(await screen.findByRole("button", { name: "Új kapcsolat" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Név" }), {
    target: { value: "Könyvelő API" },
  });
  expect(
    (screen.getByRole("textbox", { name: /^Rövid név/ }) as HTMLInputElement)
      .value,
  ).toBe("konyvelo_api");
  fireEvent.change(screen.getByRole("combobox", { name: "Hitelesítés" }), {
    target: { value: "bearer" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Létrehozás" }));
  expect(await screen.findByText("Adj meg egy http(s) címet.")).toBeTruthy();
  expect(calls).toHaveLength(0);
  fireEvent.change(screen.getByRole("textbox", { name: /^Cím/ }), {
    target: { value: "https://books.example.com/mcp" },
  });
  fireEvent.change(screen.getByLabelText(/^Titok/), {
    target: { value: "tok-123" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Létrehozás" }));
  await waitFor(() =>
    expect(calls).toContainEqual({
      method: "POST",
      path: "/api/v1/workshop/connectors",
      body: {
        slug: "konyvelo_api",
        kind: "mcp",
        title: "Könyvelő API",
        base_url: "https://books.example.com/mcp",
        auth: "bearer",
        secret: "tok-123",
      },
    }),
  );
});

test("without the server's key only connectors without a secret can be added", async () => {
  show(true, [], false);
  expect(await screen.findByText(/COLONY_SECRET_KEYS/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Új kapcsolat" }));
  const bearer = screen.getByRole("option", {
    name: "Bearer token",
  }) as HTMLOptionElement;
  expect(bearer.disabled).toBe(true);
});
