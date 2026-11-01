import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import type { Actor } from "../model";
import ThreadsPage from "./ThreadsPage";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" } as Actor;
const bot = { ...me, id: "a1", kind: "agent", handle: "trader", display_name: "trader" } as Actor;
const msg = (id: string, author: string, body: string, extra = {}) => ({ id, channel_id: "c1", author_id: author, kind: "text", body, mentions: [], reply_count: 0, created_at: "2026-11-01T10:00:00Z", ...extra });

test("Szálaim lists the threads with their unread replies; the filter and mark-all call the API", async () => {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      calls.push(`${req.method} ${u.pathname}${u.search}`);
      if (u.pathname.endsWith("/me/threads/read")) return Response.json({ marked: 1 });
      return Response.json({
        unread_threads: 1,
        threads: [
          {
            root: msg("r1", "me", "Mikor deployolunk?", { reply_count: 3, reply_participant_ids: ["a1"] }),
            channel_id: "c1", channel_name: "ops", channel_kind: "public", unread_replies: 2, mentioned: false,
            latest_replies: [msg("x2", "a1", "Holnap reggel.", { thread_root_id: "r1" }), msg("x3", "a1", "Kész a build.", { thread_root_id: "r1" })],
          },
        ],
      });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([me, bot].map((a) => [a.id, a])));
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "x" });
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ThreadsPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(await screen.findByText("2 új válasz")).toBeTruthy();
  expect(screen.getByText("# ops")).toBeTruthy();
  expect(screen.getByText("Kész a build.")).toBeTruthy();
  expect(screen.getByText("1 korábbi válasz")).toBeTruthy();

  fireEvent.click(screen.getByRole("radio", { name: "Olvasatlan" }));
  await waitFor(() => expect(calls.some((c) => c.includes("/me/threads?") && c.includes("unread=true"))).toBe(true));
  fireEvent.click(screen.getByRole("button", { name: /Mind olvasottnak jelölése/ }));
  await waitFor(() => expect(calls).toContain("POST /api/v1/me/threads/read"));
  vi.unstubAllGlobals();
});
