// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { StorageBanner } from "./StorageBanner";

const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });

function show(st: Schemas["WorkspaceStorage"], admin = true) {
  const fetch = vi.fn(async () => json(st));
  vi.stubGlobal("fetch", fetch);
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <StorageBanner admin={admin} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return fetch;
}

const base = { quota_bytes: 10 * 1024 ** 3, attachments_bytes: 0, chat_search_bytes: 0, other_bytes: 0 };

afterEach(() => vi.unstubAllGlobals());

test("the owner sees the 80% warning with numbers, no names", async () => {
  show({ ...base, used_bytes: 8.5 * 1024 **3, state: "warning_80" });
  const banner = await screen.findByRole("status");
  expect(banner.textContent).toContain("80% fölött");
  expect(banner.textContent).toContain("8,5 GB");
  expect(banner.textContent).toContain("10 GB");
});

test("full says uploads fail and the agents are paused", async () => {
  show({ ...base, used_bytes: 10 * 1024 ** 3, state: "full" });
  expect((await screen.findByRole("status")).textContent).toContain("az agentek szünetelnek");
});

test("nothing below 80%, and a member never asks", async () => {
  show({ ...base, used_bytes: 1, state: "ok" });
  await new Promise((r) => setTimeout(r, 20));
  expect(screen.queryByRole("status")).toBeNull();
  vi.unstubAllGlobals();
  const fetch = show({ ...base, used_bytes: 10 * 1024 ** 3, state: "full" }, false);
  await new Promise((r) => setTimeout(r, 20));
  expect(fetch).not.toHaveBeenCalled();
});
