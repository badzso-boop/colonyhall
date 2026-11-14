// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { EmojiButton } from "./EmojiButton";

function setup() {
  const calls: Array<{ method: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      if (req.method === "GET") return new Response(JSON.stringify({ recent: ["🎉"], frequent: ["👍"] }), { headers: { "Content-Type": "application/json" } });
      calls.push({ method: req.method, body: await req.clone().json() });
      return new Response(null, { status: 204 });
    }),
  );
  const onPick = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <EmojiButton label="Reakció hozzáadása" onPick={onPick} />
    </QueryClientProvider>,
  );
  return { calls, onPick };
}

afterEach(() => vi.unstubAllGlobals());

test("the picker loads on open, shows recent and frequent, and records the pick", async () => {
  const { calls, onPick } = setup();
  fireEvent.click(screen.getByRole("button", { name: "Reakció hozzáadása" }));
  const recent = await screen.findByRole("region", { name: "Legutóbbiak" }, { timeout: 10_000 });
  expect(recent.textContent).toContain("🎉");
  expect(screen.getByRole("region", { name: "Gyakoriak" }).textContent).toContain("👍");
  fireEvent.change(screen.getByRole("searchbox", { name: "Emoji keresése" }), { target: { value: "hüvelykujj fel" } });
  fireEvent.click(await screen.findByRole("button", { name: "felfelé mutató hüvelykujj" }));
  expect(onPick).toHaveBeenCalledWith("👍");
  await waitFor(() => expect(calls).toEqual([{ method: "POST", body: { emoji: "👍" } }]));
});

test("Enter in the search picks the best match", async () => {
  const { onPick } = setup();
  fireEvent.click(screen.getByRole("button", { name: "Reakció hozzáadása" }));
  const search = await screen.findByRole("searchbox", { name: "Emoji keresése" }, { timeout: 10_000 });
  await screen.findByRole("region", { name: "Legutóbbiak" }, { timeout: 10_000 });
  fireEvent.change(search, { target: { value: "tada" } });
  fireEvent.keyDown(search, { key: "Enter" });
  expect(onPick).toHaveBeenCalledWith("🎉");
});
