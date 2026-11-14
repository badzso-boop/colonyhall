import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { useToasts } from "@/shared/ui";
import MyFilesPage from "./MyFilesPage";

const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

const general = { id: "c1", name: "general", kind: "public" };
const files: Schemas["MyFile"][] = [
  { id: "f1", name: "terv.pdf", mime: "application/pdf", size: 2048, created_at: "2026-11-14T09:00:00Z", channel: general, message_id: "m1", shared_count: 1 },
  { id: "f2", name: "kep.png", mime: "image/png", size: 1024, created_at: "2026-11-13T09:00:00Z", shared_count: 0 },
];

function show(answer: (path: string, url: URL, method: string) => Response | undefined = () => undefined) {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const url = new URL(req.url);
      calls.push(`${req.method} ${url.pathname}${url.search}`);
      const custom = answer(url.pathname, url, req.method);
      if (custom) return custom;
      if (url.pathname.endsWith("/me/files")) return json({ files: url.searchParams.get("uploader") === "agents" ? [] : files });
      if (req.method === "DELETE") return new Response(null, { status: 204 });
      return json({});
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <MemoryRouter>
        <MyFilesPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

test("lists my files with channel, message link, copies, and a channel I no longer see", async () => {
  show();
  const row = (await screen.findByText("terv.pdf")).closest("li")!;
  expect(within(row).getByText("#general")).toBeTruthy();
  expect(within(row).getByText("még 1 példány")).toBeTruthy();
  expect(within(row).getByRole("link", { name: /Üzenet/ }).getAttribute("href")).toBe("/c/c1");
  const other = screen.getByText("kep.png").closest("li")!;
  expect(within(other).getByText("Már nem látott csatorna")).toBeTruthy();
  expect(within(other).queryByRole("link", { name: /Üzenet/ })).toBeNull();
});

test("the agents' tab asks for uploader=agents", async () => {
  const calls = show();
  await screen.findByText("terv.pdf");
  fireEvent.click(screen.getByRole("tab", { name: "Agentjeim feltöltései" }));
  await screen.findByText("Az agentjeidnek itt nincs fájlja.");
  expect(calls.some((c) => c.includes("/me/files?uploader=agents"))).toBe(true);
});

test("deleting selected files asks first, says what happens to the messages, and deletes each", async () => {
  const calls = show();
  await screen.findByText("terv.pdf");
  const del = screen.getByRole("button", { name: "Törlés" });
  expect((del as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("checkbox", { name: "terv.pdf kijelölése" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "kep.png kijelölése" }));
  fireEvent.click(del);
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("„Fájl törölve”");
  expect(dialog.textContent).toContain("a többi példány megmarad");
  fireEvent.click(within(dialog).getByRole("button", { name: "2 fájl törlése" }));
  await waitFor(() => expect(calls.filter((c) => c.startsWith("DELETE")).length).toBe(2));
  expect(calls).toContain("DELETE /api/v1/files/f1");
  expect(calls).toContain("DELETE /api/v1/files/f2");
});

test("a file someone may not delete stays selected and is reported", async () => {
  show((path, _url, method) => (method === "DELETE" && path.endsWith("/f1") ? json({ status: 403, title: "Forbidden" }, 403) : undefined));
  await screen.findByText("terv.pdf");
  fireEvent.click(screen.getByRole("checkbox", { name: "terv.pdf kijelölése" }));
  fireEvent.click(screen.getByRole("button", { name: "Törlés" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "1 fájl törlése" }));
  await waitFor(() => expect(useToasts.getState().items.map((i) => i.text)).toContain("1 fájlt nem sikerült törölni."));
  expect(screen.getByRole("checkbox", { name: "terv.pdf kijelölése" }).getAttribute("aria-checked")).toBe("true");
});
