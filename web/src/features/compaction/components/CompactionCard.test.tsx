// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { dispatch } from "@/shared/api";
import { registerCompactionEvents } from "../events";
import { useLiveCompactions } from "../store";
import { CompactionCard, CompactionLive } from "./CompactionCard";

const summary = "# Összefoglaló\n\n## Döntések\n- Postgres marad\n- a 34. kör kész";

function setup(ui: React.ReactNode, status = { has_pending_compaction: false }) {
  const calls: Array<{ method: string; path: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      calls.push({ method: req.method, path: u.pathname });
      if (u.pathname.endsWith("/content")) return new Response(summary);
      const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { "Content-Type": "application/json" } });
      if (u.pathname.endsWith("/compaction")) return json({ channel_id: "c1", approx_tokens_since: 10, messages_since: 1, threshold_tokens: 350000, ...status });
      return json({ id: "k2", workspace_id: "w", channel_id: "c1", trigger: "regenerate", state: "pending", created_at: "" });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const off = registerCompactionEvents(qc);
  render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
  return { calls, off };
}

afterEach(() => {
  vi.unstubAllGlobals();
  useLiveCompactions.setState({ live: {} });
});

test("the card previews the summary, opens it in a Sheet and regenerates after a confirmation", async () => {
  const { calls, off } = setup(
    <>
      <CompactionCard compactionId="k1" attachments={[{ id: "f1", name: "summary.md", mime: "text/markdown" }]} canWrite />
      <CompactionLive channelId="c1" human />
    </>,
  );
  const card = screen.getByRole("region", { name: "Tömörítés összefoglalója" });
  await waitFor(() => expect(within(card).getByText("Postgres marad", { exact: false })).toBeTruthy());
  expect(within(card).getByText("Összefoglaló")).toBeTruthy();

  fireEvent.click(within(card).getByRole("button", { name: "Összefoglaló megnyitása" }));
  const sheet = await screen.findByRole("dialog");
  expect(within(sheet).getByRole("heading", { name: "Döntések" })).toBeTruthy();
  fireEvent.keyDown(sheet, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

  fireEvent.click(within(card).getByRole("button", { name: "Újragenerálás" }));
  const confirm = await screen.findByRole("dialog", { name: "Újragenerálod az összefoglalót?" });
  fireEvent.click(within(confirm).getByRole("button", { name: "Újragenerálás" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.path === "/api/v1/compactions/k1/regenerate")).toBe(true));
  // The new compaction is pending: the spinner card shows until its event.
  expect(await screen.findByText("Tömörítés folyamatban…")).toBeTruthy();
  off();
});

test("without write access there is no edit or regenerate", async () => {
  const { off } = setup(<CompactionCard compactionId="k1" attachments={[{ id: "f1", name: "summary.md", mime: "text/markdown" }]} canWrite={false} />);
  await screen.findByText("Postgres marad", { exact: false });
  expect(screen.queryByRole("button", { name: "Szerkesztés" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Újragenerálás" })).toBeNull();
  off();
});

test("live: created shows a spinner, failed an error card for humans, done removes it", async () => {
  const { off } = setup(<CompactionLive channelId="c1" human />);
  const ev = (type: string, data: unknown) => act(() => dispatch({ type: "event", event_type: type, data }));
  ev("compaction.created", { compaction_id: "k9", channel_id: "c1" });
  expect(await screen.findByText("Tömörítés folyamatban…")).toBeTruthy();
  ev("compaction.failed", { compaction_id: "k9", channel_id: "c1", error_code: "scribe_timeout" });
  const alert = await screen.findByRole("alert");
  expect(alert.textContent).toContain("A tömörítés nem sikerült.");
  expect(alert.textContent).toContain("[scribe_timeout]");
  ev("compaction.created", { compaction_id: "k10", channel_id: "c1", thread_root_id: "t1" });
  ev("compaction.done", { compaction_id: "k10", channel_id: "c1" });
  expect(screen.queryByText("Tömörítés folyamatban…")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Elrejtés" }));
  expect(screen.queryByRole("alert")).toBeNull();
  off();
});

test("a compaction running since before the page loaded shows from the counter", async () => {
  const { off } = setup(<CompactionLive channelId="c1" human />, { has_pending_compaction: true });
  expect(await screen.findByText("Tömörítés folyamatban…")).toBeTruthy();
  off();
});
