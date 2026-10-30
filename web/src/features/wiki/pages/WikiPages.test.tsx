import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { identityKeys } from "@/features/identity";
import WikiIndexPage from "./WikiIndexPage";
import WikiTopicPage from "./WikiTopicPage";

const actor = (id: string, kind: string, handle: string) => ({ id, kind, handle, display_name: handle, role: "owner", state: "active", created_at: "" });
const me = actor("h", "human", "norbi");
const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const idx = (slug: string, title: string, tags: string[], updated: string, extra = {}) => ({
  slug, title, tags, excerpt: `${title} kivonat`, created_at: "2026-10-01T00:00:00Z", updated_at: updated, created_by: "a", post_count: 3, participant_ids: ["b", "a"], ...extra,
});
const b1 = "Nyitó bejegyzés.";
const b2 = `${b1}\n\n---\n*@backend, 2026-10-30 09:00:*\n\nElső válasz.`;

function setup(path: string) {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      const body = req.method === "POST" || req.method === "PUT" ? await req.json() : null;
      calls.push({ method: req.method, path: u.pathname + u.search, body });
      if (u.pathname.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      if (u.pathname === "/api/v1/wiki")
        return json({ pages: [idx("deploy", "Élesítés", ["ops"], "2026-10-29T10:00:00Z"), idx("db", "Adatbázis", ["ops", "db"], "2026-10-30T10:00:00Z"), idx("old", "Régi", [], "2026-10-02T00:00:00Z", { deleted_at: "2026-10-03T00:00:00Z" })] });
      if (u.pathname.endsWith("/wiki/db/revisions"))
        return json({ revisions: [{ page_id: "p", rev: 1, kind: "create", body: b1, author_id: "a", created_at: "2026-10-29T08:00:00Z" }, { page_id: "p", rev: 2, kind: "append", body: b2, author_id: "b", created_at: "2026-10-30T09:00:00Z" }] });
      if (u.pathname.endsWith("/wiki/db")) return json({ id: "p", slug: "db", title: "Adatbázis", tags: ["ops"], body: b2, rev: 2, created_by: "a", created_at: "", updated_at: "2026-10-30T09:00:00Z" });
      return json({ id: "p", slug: "db", title: "x", tags: [], body: "", rev: 3, created_by: "a", created_at: "", updated_at: "" });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["h", me], ["a", actor("a", "agent", "frontend")], ["b", actor("b", "agent", "backend")]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/wiki" element={<WikiIndexPage />} />
          <Route path="/wiki/:slug" element={<WikiTopicPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

test("the index: tags with counts, newest activity first, a tag filters, deleted ones apart", async () => {
  setup("/wiki");
  const links = await screen.findAllByRole("link");
  expect(links.map((l) => l.textContent)).toEqual(["Adatbázis", "Élesítés"]);
  const tags = screen.getByRole("navigation", { name: "Címkék" });
  expect(within(tags).getAllByRole("button").map((b) => b.textContent)).toEqual(["Összes2", "#ops2", "#db1"]);
  fireEvent.click(within(tags).getByRole("button", { name: /#db/ }));
  expect(screen.getAllByRole("link").map((l) => l.textContent)).toEqual(["Adatbázis"]);
  fireEvent.change(screen.getByRole("combobox", { name: "Rendezés" }), { target: { value: "title" } });
  fireEvent.click(within(tags).getByRole("button", { name: /Összes/ }));
  expect(screen.getAllByRole("link").map((l) => l.textContent)).toEqual(["Adatbázis", "Élesítés"]);
  fireEvent.click(screen.getByRole("button", { name: "Törölt" }));
  expect(await screen.findByRole("link", { name: "Régi" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Visszaállítás" })).toBeTruthy();
});

test("the topic: posts from the revisions, quote into the reply, append", async () => {
  const calls = setup("/wiki/db");
  expect(await screen.findByText("Nyitó bejegyzés.")).toBeTruthy();
  const reply = await screen.findByRole("article", { name: "@backend" });
  expect(within(reply).getByText("Első válasz.")).toBeTruthy();
  expect(within(reply).queryByText(/2026-10-30 09:00/)).toBeNull();
  fireEvent.click(within(reply).getByRole("button", { name: "Idézet" }));
  const box = screen.getByRole("textbox", { name: "Válasz" }) as HTMLTextAreaElement;
  expect(box.value).toBe("> @backend:\n> Első válasz.\n\n");
  fireEvent.change(box, { target: { value: "Köszi!" } });
  fireEvent.click(screen.getByRole("button", { name: "Küldés" }));
  await waitFor(() => expect(calls.find((c) => c.path.endsWith("/append"))?.body).toEqual({ text: "Köszi!" }));
  expect(JSON.parse(localStorage.getItem("colony.wiki.seen")!)).toEqual({ db: "2026-10-30T09:00:00Z" });
});
