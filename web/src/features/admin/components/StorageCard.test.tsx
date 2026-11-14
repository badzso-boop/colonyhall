import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Schemas } from "@/shared/api";
import { gbToBytes, storagePercent } from "../model";
import { StorageCard } from "./StorageCard";

const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });
const GB = 1024 ** 3;
const st: Schemas["WorkspaceStorage"] = {
  quota_bytes: 20 * GB, used_bytes: 19.2 * GB, attachments_bytes: 15 * GB, chat_search_bytes: 3 * GB, other_bytes: 1.2 * GB,
  db_measured_at: "2026-11-14T09:00:00Z", state: "warning_95",
};

function show(role: "owner" | "admin") {
  const calls: Array<{ method: string; body: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (path.endsWith("/auth/session")) return json({ actor: { id: "me", kind: "human", handle: "me", display_name: "Me", role, state: "active", created_at: "" } });
      if (path.endsWith("/workspace/storage/quota")) {
        const body = (await req.json()) as { quota_bytes: number };
        calls.push({ method: req.method, body });
        return json({ ...st, quota_bytes: body.quota_bytes, state: "ok" });
      }
      if (path.endsWith("/workspace/storage")) return json(st);
      return json({});
    }),
  );
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <StorageCard />
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

test("the card shows the usage by kind, the state and the meter; never people", async () => {
  show("admin");
  const meter = await screen.findByRole("meter");
  expect(meter.getAttribute("aria-valuenow")).toBe("96");
  const card = meter.closest("section")!;
  expect(card.textContent).toContain("95% fölött");
  expect(card.textContent).toContain("19,2 GB / 20 GB (96%)");
  expect(card.textContent).toContain("Csatolmányok15 GB");
  expect(card.textContent).toContain("Chat- és keresési adatok3 GB");
  // An admin sees the quota, only the owner sets it.
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(card.textContent).toContain("Keret20 GB");
});

test("the owner sets the quota in GB; empty is unlimited", async () => {
  const calls = show("owner");
  const input = (await screen.findByRole("textbox", { name: "Keret (GB)" })) as HTMLInputElement;
  expect(input.value).toBe("20");
  fireEvent.change(input, { target: { value: "50,5" } });
  fireEvent.click(screen.getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(calls).toEqual([{ method: "PUT", body: { quota_bytes: Math.round(50.5 * GB) } }]));
  fireEvent.change(input, { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(calls[1]).toEqual({ method: "PUT", body: { quota_bytes: 0 } }));
  fireEvent.change(input, { target: { value: "sok" } });
  expect((screen.getByRole("button", { name: "Mentés" }) as HTMLButtonElement).disabled).toBe(true);
});

test("quota field and percent", () => {
  expect(gbToBytes("")).toBe(0);
  expect(gbToBytes(" 0 ")).toBe(0);
  expect(gbToBytes("15")).toBe(15 * GB);
  expect(gbToBytes("0.5")).toBe(GB / 2);
  expect(gbToBytes("-1")).toBeNull();
  expect(gbToBytes("1e3")).toBeNull();
  expect(storagePercent(5, 0)).toBeNull();
  expect(storagePercent(80, 100)).toBe(80);
});
