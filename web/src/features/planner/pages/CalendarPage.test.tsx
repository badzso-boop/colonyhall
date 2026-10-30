import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { installShortcuts } from "@/shared/lib";
import CalendarPage from "./CalendarPage";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const cal = { id: "c1", name: "Munka", url_masked: "https://cal.example/…/x.ics", colour: "#2563eb", last_error: "", created_at: "2026-10-01T00:00:00Z", last_fetched_at: "2026-10-30T08:00:00Z" };
const ev = (title: string, start: Date, end: Date, all_day = false) => ({ calendar_id: "c1", title, start: start.toISOString(), end: end.toISOString(), all_day, location: "Iroda", description: "" });
const at = (d: number, h: number) => new Date(2026, 9, d, h);

function setup(calendars = [cal]) {
  const asked: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const url = new URL(req.url);
      if (url.pathname.endsWith("/planner/calendars")) return json({ calendars });
      if (url.pathname.endsWith("/planner/events")) {
        asked.push(url);
        return json({
          events: [
            ev("Állófogadás", at(30, 9), at(30, 10)),
            ev("Tervezés", at(30, 11), at(30, 12)),
            ev("Ebéd", at(30, 12), at(30, 13)),
            ev("Szünnap", new Date(2026, 9, 30), new Date(2026, 9, 31), true),
          ],
        });
      }
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <CalendarPage />
    </QueryClientProvider>,
  );
  return asked;
}

let off = () => {};
beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 9, 30, 8, 0));
  off = installShortcuts();
});
afterEach(() => {
  off();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  localStorage.clear();
});

test("the month: a Monday-first 6-week grid, at most three lines a day and +N more", async () => {
  const asked = setup();
  expect(screen.getByRole("heading", { name: "2026. október" })).toBeTruthy();
  expect(await screen.findByText("Szünnap")).toBeTruthy();
  expect(screen.getByText("Állófogadás")).toBeTruthy();
  expect(screen.queryByText("Ebéd")).toBeNull();
  expect(screen.getByRole("button", { name: "+2 további" })).toBeTruthy();
  expect(asked[0]!.searchParams.get("from")).toBe(new Date(2026, 8, 28).toISOString());
  fireEvent.click(screen.getByRole("button", { name: /október 30\., péntek megnyitása/ }));
  expect(screen.getByRole("radio", { name: "Nap" }).getAttribute("aria-checked")).toBe("true");
});

test("keys switch the view and move the range; the week asks Monday to Monday", async () => {
  const asked = setup();
  await screen.findByText("Szünnap");
  fireEvent.keyDown(window, { key: "w" });
  await waitFor(() => expect(asked.at(-1)!.searchParams.get("from")).toBe(new Date(2026, 9, 26).toISOString()));
  expect(asked.at(-1)!.searchParams.get("to")).toBe(new Date(2026, 10, 2).toISOString());
  fireEvent.keyDown(window, { key: "ArrowRight" });
  await waitFor(() => expect(asked.at(-1)!.searchParams.get("from")).toBe(new Date(2026, 10, 2).toISOString()));
  fireEvent.keyDown(window, { key: "t" });
  fireEvent.keyDown(window, { key: "d" });
  expect(screen.getByRole("heading", { name: "okt. 30., péntek" })).toBeTruthy();
});

test("no calendars: an empty state that says events cannot be created here", async () => {
  setup([]);
  expect(await screen.findByText("Még nincs naptárad")).toBeTruthy();
  expect(screen.getByText(/eseményt itt nem lehet létrehozni/)).toBeTruthy();
});
