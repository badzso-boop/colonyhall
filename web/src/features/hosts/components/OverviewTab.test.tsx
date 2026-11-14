// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { Schemas } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import { OverviewTab } from "./OverviewTab";

type Host = Schemas["Host"];

const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });

function show(report: Partial<Host["report"]>) {
  const h: Host = { id: "h1", name: "pc-dev", color: "blue", state: "approved", created_at: "", report: { roots: [], clients: [], rejected: [], ...report }, agents: [] };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => json({ actor: { id: "norbi", kind: "human", handle: "norbi", display_name: "norbi", role: "owner", state: "active", created_at: "" }, csrf_token: "x" })),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map());
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <OverviewTab host={h} now={new Date("2026-11-08T12:00:00Z")} />
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

// Round 39: the Machines page shows CodeGraph's version and the indexed repositories.
test("CodeGraph: version and repositories with their state", () => {
  show({
    codegraph: {
      version: "1.6.2",
      repos: [
        { path: "/home/alice/projects/colonyhall", status: "indexed", files: 807, indexed_at: "2026-11-08T10:00:00Z" },
        { path: "/home/alice/projects/huge", status: "skipped", files: 90000, detail: "90000 files, more than codegraph.max_files (20000)" },
      ],
    },
  });
  expect(screen.getByText("CodeGraph")).toBeTruthy();
  expect(screen.getByText("1.6.2 verzió")).toBeTruthy();
  expect(screen.getByText("/home/alice/projects/colonyhall")).toBeTruthy();
  expect(screen.getByText("indexelve")).toBeTruthy();
  expect(screen.getByText("807 fájl")).toBeTruthy();
  expect(screen.getByText("kihagyva")).toBeTruthy();
  expect(screen.getByText(/more than codegraph\.max_files/)).toBeTruthy();
});

test("CodeGraph: absent when the guard has none", () => {
  show({});
  expect(screen.getByText("nincs (nincs telepítve, vagy a guard.yaml kikapcsolja)")).toBeTruthy();
});

test("CodeGraph: installed, nothing indexed yet", () => {
  show({ codegraph: { version: "1.6.2", repos: [] } });
  expect(screen.getByText("még nincs indexelt repó")).toBeTruthy();
});
