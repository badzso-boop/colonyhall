import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import type { Schemas } from "@/shared/api";
import { TooltipProvider } from "@/shared/ui";
import { identityKeys } from "@/features/identity";
import type { Message } from "../api";
import { MessageItem, type Viewer } from "./MessageItem";

const errId = "0b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f21";
const apId = "1b5c2a8e-1f2d-4c3b-9a8e-7d6c5b4a3f22";
const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" } satisfies Schemas["Actor"];
const agent = { id: "ag", kind: "agent", handle: "backend-claude", display_name: "Backend", role: "member", state: "active", created_at: "" } satisfies Schemas["Actor"];
const actors = new Map<string, Schemas["Actor"]>([[me.id, me], [agent.id, agent]]);
const handles = new Set(["norbi", "backend-claude"]);
const human: Viewer = { id: "me", human: true, admin: true, canWrite: true, pausedAgents: new Set() };

const msg = (over: Partial<Message>): Message => ({
  id: "m1", channel_id: "c1", author_id: "ag", kind: "text", body: "", mentions: [], reply_count: 0, created_at: new Date().toISOString(), ...over,
});

function setup(m: Message, viewer: Viewer = human, extra: { onOpenTerminal?: (a: string, n: string) => void } = {}) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
      calls.push({ method: req.method, path: u.pathname, body: req.method === "GET" ? undefined : await req.clone().json().catch(() => null) });
      const p = u.pathname;
      if (p.endsWith("/runs"))
        return json({
          runs: [
            {
              id: "r1", workspace_id: "w", agent_id: "ag", phase: "direct", client: "claude", model: "opus", session_id: "s", items: 1, input_tokens: 1, output_tokens: 1,
              cache_read_tokens: 0, cache_write_tokens: 0, duration_ms: 192_000, status: "ok", created_at: "", reply_message_id: "m1",
              steps: [
                { kind: "read", label: "Read", path: "internal/a.go", state: "done", started_at: "", duration_ms: 1200 },
                { kind: "command", label: "go test ./...", state: "done", started_at: "" },
              ],
            },
          ],
        });
      if (p.endsWith(`/errors/${errId}`)) return json({ id: errId, agent_id: "ag", channel_id: "c1", summary: "claude exited 1", detail: "stack trace here", created_at: new Date().toISOString() });
      if (p.endsWith(`/approvals/${apId}`))
        return json({ id: apId, agent_id: "ag", channel_id: "c1", tool_name: "Bash", input: "{}", state: "pending", created_at: "", expires_at: new Date(Date.now() + 60_000).toISOString() });
      if (p.endsWith("/delegations")) return json({ delegations: [] });
      if (p.endsWith("/file-fetch")) return json({ id: "ff1", workspace_id: "w", channel_id: "c1", agent_id: "ag", requested_by: "me", path: "/srv/app/README.md", state: "done", created_at: "", file: { id: "f9", name: "README.md", mime: "text/markdown", size: 20, fetched_at: "" } }, 201);
      if (p.endsWith("/files/f9/content")) return new Response("# Olvass el\n\nszöveg");
      if (p.endsWith("/retract") || p.endsWith("/pin")) return new Response(null, { status: 204 });
      if (p.includes("/reactions/")) return new Response(null, { status: 204 });
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, actors);
  const utils = render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TooltipProvider>
          <MessageItem message={m} actors={actors} handles={handles} viewer={viewer} locale="hu-HU" summary {...extra} />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { calls, ...utils };
}

afterEach(() => vi.unstubAllGlobals());

test("chips: Q-12, D-3 and the error, whose details open for humans", async () => {
  setup(msg({ body: `Lásd Q-12 és D-3. I ran into an error [error:${errId}]` }));
  expect(screen.getByRole("button", { name: "Q-12" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "D-3" })).toBeTruthy();
  expect(screen.queryByText(errId, { exact: false })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "hiba" }));
  const sheet = await screen.findByRole("dialog", { name: "A hiba részletei" });
  await waitFor(() => expect(within(sheet).getByText("stack trace here")).toBeTruthy());
});

test("an agent viewer gets no error details, approvals or steps", async () => {
  const agentViewer: Viewer = { id: "other", human: false, admin: false, canWrite: true, pausedAgents: new Set() };
  const { calls } = setup(msg({ body: `oops [error:${errId}] [approval:${apId}]` }), agentViewer);
  expect(screen.queryByRole("button", { name: "hiba" })).toBeNull();
  expect(screen.getByText("hiba")).toBeTruthy();
  expect(screen.queryByRole("region")).toBeNull();
  expect(screen.queryByText("approval", { exact: false })).toBeNull();
  await new Promise((r) => setTimeout(r, 20));
  expect(calls.some((c) => c.path.endsWith("/runs") || c.path.includes("/approvals/"))).toBe(false);
});

test("an approval reference becomes a card under the message", async () => {
  setup(msg({ body: `Engedélyt kérek. [approval:${apId}]` }));
  expect(await screen.findByRole("region", { name: "Engedélykérés: Bash" })).toBeTruthy();
  expect(screen.getByText("Engedélyt kérek.")).toBeTruthy();
});

test("a terminal notice opens the terminal; the step drawer lists the run's steps", async () => {
  const onOpenTerminal = vi.fn();
  setup(msg({ body: "⚙️ build kész (exit 0)" }), human, { onOpenTerminal });
  fireEvent.click(screen.getByRole("button", { name: "Terminál megtekintése" }));
  expect(onOpenTerminal).toHaveBeenCalledWith("ag", "build");
  const drawer = await screen.findByRole("button", { name: "2 lépés · 3:12" });
  expect(drawer.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(drawer);
  expect(screen.getByText("internal/a.go")).toBeTruthy();
  expect(screen.getByText("go test ./...")).toBeTruthy();
  expect(screen.getByText("1.2 s")).toBeTruthy();
});

test("Gitea cards, reply routing links and the paused badge", () => {
  setup(
    msg({
      body: "kész",
      link_cards: [{ url: "https://gitea.example/team/app/pulls/7", kind: "pull", title: "Add login", state: "open", author: "colony-agents" }],
      origin: { channel_id: "c9", channel: "general", thread_id: "t9" },
      answers: { message_id: "x", channel_id: "dm1", author_id: "me" },
    }),
    { ...human, pausedAgents: new Set(["ag"]) },
  );
  const card = screen.getByRole("link", { name: /Add login/ });
  expect(card.getAttribute("href")).toBe("https://gitea.example/team/app/pulls/7");
  expect(card.getAttribute("rel")).toBe("noopener noreferrer");
  expect(card.textContent).toContain("team/app");
  expect(card.textContent).toContain("nyitott");
  expect(screen.getByRole("link", { name: /az eredeti szálban: #general/ }).getAttribute("href")).toBe("/c/c9?thread=t9");
  expect(screen.getByRole("link", { name: /válasz @norbi DM-jére/ }).getAttribute("href")).toBe("/c/dm1");
  expect(screen.getByText("⏸ szünetel")).toBeTruthy();
});

test("a rate-limit note is a muted line", () => {
  setup(msg({ body: "⏸ A Claude 5 órás kerete 95%-on, 17:25-kor folytatom." }));
  const note = screen.getByRole("note");
  expect(note.textContent).toContain("@backend-claude");
  expect(screen.queryByRole("article")).toBeNull();
});

test("a local file link opens the fetched file in a Sheet", async () => {
  const { calls } = setup(msg({ body: "Megírtam: /srv/app/README.md" }));
  fireEvent.click(screen.getByRole("button", { name: /README.md/ }));
  const sheet = await screen.findByRole("dialog", { name: "README.md" });
  await waitFor(() => expect(within(sheet).getByRole("heading", { name: "Olvass el" })).toBeTruthy());
  expect(calls.find((c) => c.method === "POST")).toEqual({ method: "POST", path: "/api/v1/messages/m1/file-fetch", body: { path: "/srv/app/README.md", refresh: false } });
});

test("retract from the message menu, confirmed in place; a retracted message shows 🗑", async () => {
  const { calls, rerender } = setup(msg({ author_id: "me", body: "elírtam" }));
  fireEvent.keyDown(screen.getByRole("button", { name: "További műveletek" }), { key: "Enter" });
  fireEvent.click(await screen.findByRole("menuitem", { name: "Visszavonás" }));
  const confirm = screen.getByRole("group", { name: /Biztosan visszavonod/ });
  fireEvent.click(within(confirm).getByRole("button", { name: "Visszavonás" }));
  await waitFor(() => expect(calls.some((c) => c.method === "POST" && c.path === "/api/v1/messages/m1/retract")).toBe(true));
  rerender(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <TooltipProvider>
          <MessageItem message={msg({ author_id: "me", body: "elírtam", deleted_at: new Date().toISOString() })} actors={actors} handles={handles} viewer={human} locale="hu-HU" summary />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(screen.getByText(/Visszavonva/).textContent).toContain("🗑");
  expect(screen.queryByText("elírtam")).toBeNull();
});

test("someone else's message has no retract; reactions toggle with their meaning as the name", async () => {
  const { calls } = setup(msg({ body: "javaslat", reactions: [{ emoji: "👍", count: 1, actor_ids: ["me"] }] }));
  fireEvent.keyDown(screen.getByRole("button", { name: "További műveletek" }), { key: "Enter" });
  await screen.findByRole("menuitem", { name: "Kitűzés" });
  expect(screen.queryByRole("menuitem", { name: "Visszavonás" })).toBeNull();
  fireEvent.keyDown(document.activeElement!, { key: "Escape" });
  fireEvent.click(screen.getByRole("button", { name: /👍 egyetértés/ }));
  fireEvent.click(screen.getByRole("button", { name: /👎 nem ért egyet/ }));
  await waitFor(() => expect(calls.filter((c) => c.path.includes("/reactions/")).map((c) => c.method)).toEqual(["DELETE", "PUT"]));
});

test("pin and unpin from the message menu; an agent's important message is marked", async () => {
  const { calls, rerender } = setup(msg({ body: "kész a deploy", important: true }));
  expect(screen.getByText("Fontos")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("button", { name: "További műveletek" }), { key: "Enter" });
  fireEvent.click(await screen.findByRole("menuitem", { name: "Kitűzés" }));
  await waitFor(() => expect(calls.some((c) => c.method === "PUT" && c.path === "/api/v1/messages/m1/pin")).toBe(true));
  rerender(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <TooltipProvider>
          <MessageItem message={msg({ body: "kész a deploy", pinned: true })} actors={actors} handles={handles} viewer={human} locale="hu-HU" summary />
        </TooltipProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  expect(screen.getByLabelText("Kitűzött üzenet")).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("button", { name: "További műveletek" }), { key: "Enter" });
  fireEvent.click(await screen.findByRole("menuitem", { name: "Kitűzés megszüntetése" }));
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.path === "/api/v1/messages/m1/pin")).toBe(true));
});

test("a read-only viewer gets no pin action", () => {
  setup(msg({ body: "x" }), { ...human, canWrite: false });
  expect(screen.queryByRole("button", { name: "További műveletek" })).toBeNull();
});
