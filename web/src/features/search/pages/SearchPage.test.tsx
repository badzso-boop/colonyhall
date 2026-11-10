import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import SearchPage from "./SearchPage";

const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });

const hits: Schemas["SearchHit"][] = [
  {
    kind: "message", source_id: "00000000-0000-0000-0000-0000000000a1", part: 0, channel_id: "c1", thread_root_id: "t1", at: "2026-11-05T10:00:00Z",
    header: "#fejlesztes · szál: „Postgres” · @norbi · 2026-11-05", snippet: "a «postgres» frissítése", scores: { keyword: 0.12, semantic: 0.81, fused: 0.032, rerank: 0.9 },
  },
  { kind: "wiki", source_id: "00000000-0000-0000-0000-0000000000b1", ref: "deploy", part: 1, at: "2026-11-04T10:00:00Z", header: "wiki: deploy", snippet: "telepítés lépései" },
  { kind: "decision", source_id: "00000000-0000-0000-0000-0000000000c1", ref: "D-12", part: 0, at: "2026-11-03T10:00:00Z", header: "D-12", snippet: "«Postgres» 16" },
  { kind: "message", source_id: "00000000-0000-0000-0000-0000000000a2", part: 0, channel_id: "c2", at: "2026-11-02T10:00:00Z", header: "#altalanos · @bot", snippet: "másik" },
];

function setup(path: string, respond: (u: URL) => Schemas["SearchResponse"]) {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const u = new URL(req.url);
      asked.push(u);
      return json(respond(u));
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter initialEntries={[path]}>
        <SearchPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return asked;
}

afterEach(() => vi.unstubAllGlobals());

test("results grouped by kind, highlighted, linked, with scores only where present", async () => {
  const asked = setup("/search?q=postgres", () => ({ mode: "hybrid", hits, degraded: [] }));
  const messages = await screen.findByRole("region", { name: /Üzenetek/ });
  expect(asked[0]!.searchParams.get("q")).toBe("postgres");
  expect(asked[0]!.searchParams.get("mode")).toBe("hybrid");
  expect(screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent)).toEqual(["Üzenetek2 találat", "Wiki1 találat", "Döntések1 találat"]);
  expect(within(messages).getAllByRole("article")).toHaveLength(2);

  const first = screen.getByRole("article", { name: hits[0]!.header });
  expect(within(first).getByText("postgres").tagName).toBe("MARK");
  expect(first.textContent).not.toContain("«");
  expect(within(first).getByLabelText("Pontszámok").textContent).toBe("kulcsszó 0.12 · jelentés 0.81 · RRF 0.032 · rerank 0.9");
  expect(within(first).getByRole("link").getAttribute("href")).toBe("/c/c1?thread=t1");

  const wiki = screen.getByRole("article", { name: "wiki: deploy" });
  expect(within(wiki).queryByLabelText("Pontszámok")).toBeNull();
  expect(within(wiki).queryByRole("mark")).toBeNull();
  expect(within(wiki).getByRole("link").getAttribute("href")).toBe("/wiki/deploy");
  expect(within(screen.getByRole("article", { name: "D-12" })).getByRole("link").getAttribute("href")).toBe("/decisions#D-12");
  expect(screen.queryByRole("note")).toBeNull();
});

test("the notice is a warning line, and the mode switch searches again", async () => {
  const asked = setup("/search?q=pgx", (u) => ({
    mode: (u.searchParams.get("mode") ?? "hybrid") as Schemas["SearchResponse"]["mode"],
    hits: [],
    degraded: ["no_embedder"],
    notice: "The embedding server did not answer; keyword search only.",
  }));
  expect((await screen.findByRole("note")).textContent).toContain("keyword search only");
  expect(screen.getByText("Nincs találat erre: „pgx”.")).toBeTruthy();
  expect(screen.getByRole("radio", { name: "Hibrid" }).getAttribute("aria-checked")).toBe("true");
  fireEvent.click(screen.getByRole("radio", { name: "Kulcsszó" }));
  await waitFor(() => expect(asked.at(-1)!.searchParams.get("mode")).toBe("keyword"));
  expect(screen.getByRole("radio", { name: "Kulcsszó" }).getAttribute("aria-checked")).toBe("true");
});

test("without a query nothing is asked; submitting the field searches", async () => {
  const asked = setup("/search", () => ({ mode: "hybrid", hits: [hits[1]!], degraded: [] }));
  expect(screen.getByText(/Írd be, mit keresel/)).toBeTruthy();
  expect(asked).toHaveLength(0);
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "telepítés" } });
  fireEvent.submit(screen.getByRole("search"));
  expect(await screen.findByRole("article", { name: "wiki: deploy" })).toBeTruthy();
  expect(asked[0]!.searchParams.get("q")).toBe("telepítés");
});
