// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MessageBlocks } from "./MessageBlocks";

const json = (b: unknown) =>
  new Response(JSON.stringify(b), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

function show(canAct: boolean) {
  const calls: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      calls.push({
        path: new URL(req.url).pathname,
        body: await req.json().catch(() => null),
      });
      return json({ id: "a1", label: "Rendben" });
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MessageBlocks
        messageId="m1"
        body="Mehet?"
        canAct={canAct}
        blocks={[
          { type: "text", text: "Mehet?" },
          {
            type: "buttons",
            buttons: [{ action_id: "ok", label: "Rendben", style: "primary" }],
          },
          { type: "table", columns: ["key"], rows: [["PROJ-1"]] },
        ]}
      />
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

test("a button sends the click; the body is not repeated; a table shows", async () => {
  const calls = show(true);
  expect(screen.queryAllByText("Mehet?")).toHaveLength(0);
  expect(screen.getByText("PROJ-1")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Rendben" }));
  await waitFor(() =>
    expect(calls).toContainEqual({
      path: "/api/v1/messages/m1/actions",
      body: { action_id: "ok" },
    }),
  );
  expect(await screen.findByText("Válaszod: Rendben")).toBeTruthy();
});

test("an agent viewing it cannot click", () => {
  show(false);
  expect(
    screen.getByRole("button", { name: "Rendben" }).hasAttribute("disabled"),
  ).toBe(true);
});
