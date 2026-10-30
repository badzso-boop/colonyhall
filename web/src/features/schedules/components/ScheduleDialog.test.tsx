import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import { openScheduleBuilder, useScheduleDraft } from "@/shared/lib";
import SchedulesPage from "../pages/SchedulesPage";
import { ScheduleBuilderHost } from "./ScheduleBuilderHost";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const schedule = (id: string, cron: string, state = "active", created_by = "me") => ({
  id, created_by, channel_id: "c1", body: `Jelentés ${id}\nmásodik sor`, cron, timezone: "Europe/Budapest", catch_up: "once", state,
  next_run_at: "2026-11-02T08:00:00Z", last_error: "", run_count: 3, created_at: "2026-10-01T00:00:00Z",
});

function setup(ui: React.ReactNode) {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const path = new URL(req.url).pathname;
      const body = req.method === "POST" ? await req.json() : null;
      calls.push({ method: req.method, path, body });
      if (path.endsWith("/channels")) return json({ channels: [{ id: "c1", kind: "public", name: "general", is_member: true, unread: 0, mentions: 0 }] });
      if (path.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      if (path.endsWith("/schedules/preview")) {
        if (body.cron === "bad") return json({ title: "invalid cron expression" }, 400);
        return json({ cron: body.cron ?? "", timezone: body.timezone, next_runs: ["2026-11-02T08:00:00Z", "2026-11-03T08:00:00Z"] });
      }
      if (path.endsWith("/schedules") && req.method === "POST") return json(schedule("new", body.cron ?? ""), 201);
      if (path.endsWith("/schedules")) return json({ schedules: [schedule("a", "0 9 * * 1-5"), schedule("b", "*/5 * * * *"), schedule("c", "", "done"), schedule("d", "0 7 * * *", "active", "other")] });
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["me", me]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        {ui}
        <ScheduleBuilderHost />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
  act(() => useScheduleDraft.setState({ draft: null }));
});

test("the builder: weekdays at 09:00, the server's next runs, saving sends the cron and clears the composer", async () => {
  const calls = setup(null);
  const onSaved = vi.fn();
  act(() => openScheduleBuilder({ body: "Napi jelentés", channelId: "c1", onSaved }));
  fireEvent.click(await screen.findByRole("radio", { name: "Naponta" }));
  fireEvent.click(screen.getByRole("switch"));
  expect(await screen.findByText(/Minden hétköznap 09:00-kor \(Europe\/Budapest\) a\(z\) general csatornába/)).toBeTruthy();
  await waitFor(() => expect(calls.some((c) => c.path.endsWith("/preview") && (c.body as { cron?: string }).cron === "0 9 * * 1-5")).toBe(true));
  expect(await screen.findByText("0 9 * * 1-5")).toBeTruthy();
  expect(screen.getAllByRole("listitem")).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "Ütemezés mentése" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalled());
  expect(calls.find((c) => c.method === "POST" && c.path.endsWith("/schedules"))!.body).toEqual({
    channel_id: "c1", body: "Napi jelentés", cron: "0 9 * * 1-5", timezone: "Europe/Budapest", catch_up: "once",
  });
});

test("an invalid advanced cron shows the server's reason and cannot be saved", async () => {
  setup(null);
  act(() => openScheduleBuilder({ body: "x", channelId: "c1" }));
  fireEvent.click(await screen.findByRole("radio", { name: "Haladó" }));
  fireEvent.change(screen.getByRole("textbox", { name: /Cron-kifejezés/ }), { target: { value: "bad" } });
  expect(await screen.findByText(/Ez az időzítés nem érvényes: invalid cron expression/)).toBeTruthy();
  expect((screen.getByRole("button", { name: "Ütemezés mentése" }) as HTMLButtonElement).disabled).toBe(true);
});

test("the page: own active schedules in words, admins may show everyone's, copy opens the builder prefilled", async () => {
  setup(<SchedulesPage />);
  expect(await screen.findByText("Minden hétköznap 09:00-kor")).toBeTruthy();
  expect(screen.getByText("*/5 * * * *")).toBeTruthy();
  expect(screen.queryByText("Jelentés d")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Mindenkié" }));
  expect(screen.getByText("Jelentés d")).toBeTruthy();
  fireEvent.click(screen.getByRole("radio", { name: "Kész" }));
  expect(screen.getByText("Egyszer")).toBeTruthy();
  fireEvent.click(screen.getByRole("radio", { name: "Aktív" }));
  fireEvent.click(screen.getAllByRole("button", { name: "Másolás új ütemezésbe" })[0]!);
  const dialog = await screen.findByRole("dialog");
  expect((dialog.querySelector("textarea") as HTMLTextAreaElement).value).toBe("Jelentés a\nmásodik sor");
  expect(screen.getByRole("radio", { name: "Naponta" }).getAttribute("aria-checked")).toBe("true");
});
