import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import LeasesPage from "./LeasesPage";

const me = { id: "h", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });
const at = (min: number) => new Date(Date.UTC(2026, 9, 30, 10, 0) + min * 60_000).toISOString();
const lease = (id: string, resource: string, acquired: number, expires: number, extra = {}) => ({
  id, workspace_id: "w", resource, holder_id: `a-${id}`, holder_handle: `agent-${id}`, note: `megjegyzés ${id}`, acquired_at: at(acquired), expires_at: at(expires), ...extra,
});

function setup() {
  const asked: URL[] = [];
  const posted: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      if (u.pathname.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      if (req.method === "POST") {
        posted.push(u.pathname);
        return json(lease("x", "x", 0, 1));
      }
      asked.push(u);
      const list = [lease("a", "db/migrate", -10, 3), lease("b", "repo/main", -30, 60), lease("c", "deploy", -60, -5)];
      if (u.searchParams.get("state") === "all") list.push(lease("d", "old", -600, -500, { released_at: at(-550), released_by: "h" }));
      return json({ leases: list });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["h", me]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <LeasesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { asked, posted };
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(at(0)));
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

test("countdowns, amber under five minutes, red when expired; sorting and filtering", async () => {
  setup();
  const row = (name: string) => screen.getByText(name).closest("tr")!;
  await screen.findByText("db/migrate");
  expect(within(row("db/migrate")).getByText("3:00").className).toContain("text-warning");
  expect(within(row("repo/main")).getByText("1:00:00").className).not.toContain("text-warning");
  expect(within(row("deploy")).getByText("lejárt").className).toContain("text-danger");
  const order = () => screen.getAllByRole("row").slice(1).map((r) => r.querySelector(".font-mono")!.textContent);
  expect(order()).toEqual(["deploy", "db/migrate", "repo/main"]);
  fireEvent.click(screen.getByRole("button", { name: "Rendezés: Erőforrás" }));
  expect(order()).toEqual(["db/migrate", "deploy", "repo/main"]);
  fireEvent.click(screen.getByRole("button", { name: "Rendezés: Erőforrás" }));
  expect(order()).toEqual(["repo/main", "deploy", "db/migrate"]);
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "megjegyzés b" } });
  expect(order()).toEqual(["repo/main"]);
});

test("history asks for all states; releasing names the resource and the holder", async () => {
  const { asked, posted } = setup();
  await screen.findByText("db/migrate");
  fireEvent.click(screen.getByRole("radio", { name: "Előzmények (7 nap)" }));
  expect(await screen.findByText("old")).toBeTruthy();
  expect(asked.at(-1)!.searchParams.get("state")).toBe("all");
  expect(screen.getByText("Felszabadítva")).toBeTruthy();
  fireEvent.click(within(screen.getByText("repo/main").closest("tr")!).getByRole("button", { name: "Felszabadítás" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("A(z) repo/main foglalását @agent-b tartja.");
  fireEvent.click(within(dialog).getByRole("button", { name: "Felszabadítás" }));
  await waitFor(() => expect(posted).toEqual(["/api/v1/leases/b/release"]));
});
