import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { TooltipProvider } from "@/shared/ui";
import StatusPage from "./StatusPage";

const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });

const system: NonNullable<Schemas["AdminStatus"]["system"]> = {
  version: "v1",
  uptime_seconds: 3700,
  database: { ok: true, latency_ms: 1.2 },
  event_head: 10,
  consumers: [],
  connections: { human: 1, agent: 0, host: 0 },
  guards: [],
  scheduler: { failed_24h: 0 },
  compaction: { running: 0, failed_24h: 0 },
  gitea_configured: false,
  gitea_webhook_configured: false,
  max_upload_mb: 25,
};

function show(search?: Schemas["SearchStatus"]) {
  const status: Schemas["AdminStatus"] = { emergency_stop: { active: false }, system: { ...system, ...(search ? { search } : {}) } };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (path.endsWith("/auth/session")) return json({ actor: { id: "me", kind: "human", handle: "me", display_name: "Me", role: "owner", state: "active", created_at: "2026-11-01T00:00:00Z" } });
      if (path.endsWith("/admin/status")) return json(status);
      if (path.endsWith("/actors")) return json({ actors: [] });
      return json({});
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TooltipProvider>
        <MemoryRouter>
          <StatusPage />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

test("the search card appears only when the status has a search part", async () => {
  show();
  await screen.findByRole("heading", { name: /Adatbázis/ });
  expect(screen.queryByRole("heading", { name: /^Keresés/ })).toBeNull();
});

test("the search card shows the index and the backends", async () => {
  show({ chunks: 42, pending: 0, models: ["bge-m3"], embedder: { configured: true, reachable: true, model: "bge-m3" }, reranker: { configured: false, reachable: false } });
  const heading = await screen.findByRole("heading", { name: /^Keresés/ });
  expect(heading.parentElement!.textContent).toContain("Indexelt darabok42");
});
