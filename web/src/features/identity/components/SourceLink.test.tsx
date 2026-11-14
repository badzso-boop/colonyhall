// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { SourceLink } from "./SourceLink";

const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });

afterEach(() => vi.unstubAllGlobals());

// Round 48 (AGPL section 13): the source link comes from the server, so a
// modified version's operator points it at their own source.
test("the source link shows the version and points where the server says", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) =>
      new URL(req.url).pathname.endsWith("/version")
        ? json({ version: "v1.2.3", license: "AGPL-3.0-only", source_url: "https://git.example.com/acme/colonyhall-fork" })
        : json({}),
    ),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <SourceLink />
    </QueryClientProvider>,
  );
  const link = await screen.findByRole("link", { name: "Forráskód" });
  expect(link.getAttribute("href")).toBe("https://git.example.com/acme/colonyhall-fork");
  expect(link.getAttribute("rel")).toBe("noopener noreferrer");
  expect(link.parentElement!.textContent).toContain("Colonyhall v1.2.3 · AGPL-3.0-only");
});
