import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import type { Schemas } from "@/shared/api";
import { dispatch } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import { registerHostEvents } from "../api";
import HostsPage from "./HostsPage";

type Host = Schemas["Host"];
type Actor = Schemas["Actor"];

const me: Actor = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const agentActor = (id: string, handle: string, extra: Partial<Actor> = {}): Actor => ({ id, kind: "agent", handle, display_name: handle, role: "member", state: "active", created_at: "", ...extra });
const json = (b: unknown, status = 200) => new Response(status === 204 ? null : JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const ago = (s: number) => new Date(Date.now() - s * 1000).toISOString();

const hostAgent = (id: string, handle: string, hostId: string, grants: [string, "read" | "write"][] = []): Schemas["HostAgent"] => ({
  agent_id: id, handle, host_id: hostId, client: "claude-code", model: "", work_model: "", allow_work: true, permission_mode: "auto", mode: "direct",
  root_dir: "/home/n", worktree_repo: "", sandbox: false, approvals: "chat", max_session_tokens: 0, daily_token_limit: 0,
  grants: grants.map(([path, mode], i) => ({ id: `${id}-${i}`, path, mode, created_at: "" })),
});

function hosts(): Host[] {
  return [
    {
      id: "h1", name: "pc-dev", color: "blue", state: "approved", created_at: ago(9999), last_seen_at: ago(10),
      report: {
        os: "linux", guard_version: "0.33.0", roots: ["/home/alice/projects"], clients: [{ name: "claude", version: "2.1.0" }], rejected: [],
        sandbox: "bwrap 0.9.0", exchange_dir: "/mnt/z", exchange_ok: true,
        doctor: [{ id: "claude", status: "warn", detail: "not logged in" }, { id: "tmux", status: "ok" }], doctor_checked_at: ago(60),
        worktrees: [{ agent_id: "a1", path: "/home/alice/projects/x-wt", branch: "colony/backend/work" }],
        claude_limits: { five_hour: { utilization: 0.42, status: "allowed" } },
        agy_usage: [{ name: "Gemini Models", models: "Gemini Flash, Gemini Pro", five_hour: { remaining: 0.75, resets_at: new Date(Date.now() + 3_600_000).toISOString() }, weekly: { remaining: 0.5 } }],
        agy_limits: [
          { model: "gemini-3.8-flash-high", paused_until: new Date(Date.now() + 3_600_000).toISOString(), reset_known: true },
          { model: "default", paused_until: new Date(Date.now() + 1_800_000).toISOString(), reset_known: false },
        ],
      },
      agents: [hostAgent("a1", "backend", "h1", [["/home/alice/projects/x", "write"]]), hostAgent("a2", "docs", "h1", [["/home/alice/projects/x", "read"]])],
    },
    {
      id: "h2", name: "homelab", color: "green", state: "approved", created_at: ago(9999), last_seen_at: ago(12 * 60),
      report: { os: "linux", roots: ["/srv"], clients: [], rejected: [{ agent_id: "a3", path: "/etc", reason: "outside roots" }], exchange_dir: "/srv/x", exchange_ok: false },
      agents: [],
    },
    { id: "h3", name: "old-laptop", color: "pink", state: "revoked", created_at: ago(9999), report: { roots: [], clients: [], rejected: [] }, agents: [] },
  ];
}

function Where() {
  const loc = useLocation();
  return <output data-testid="where">{loc.pathname + loc.search}</output>;
}

function setup(path = "/hosts/h1", list: () => Host[] = hosts) {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      const p = u.pathname.replace(/^\/api\/v1/, "");
      if (req.method !== "GET") calls.push({ method: req.method, path: p, body: req.method === "POST" && !req.headers.get("content-type")?.includes("json") ? null : await req.json().catch(() => null) });
      if (p.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      if (p === "/hosts" && req.method === "GET") return json({ hosts: list() });
      if (p === "/previews") return json({ previews: [] });
      if (p === "/hosts/pairing-codes") return json({ code: "K7Q2-MX4P", expires_at: new Date(Date.now() + 600_000).toISOString() }, 201);
      if (req.method === "PATCH") return json({ ...list()[0], ...(calls.at(-1)!.body as object) });
      if (req.method === "PUT") return json(null, 204);
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["me", me], ["a1", agentActor("a1", "backend")], ["a2", agentActor("a2", "docs", { color: "pink" })]]));
  const off = registerHostEvents(qc);
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[path]}>
          <Routes>
            <Route path="/hosts" element={<HostsPage />} />
            <Route path="/hosts/:hostId" element={<HostsPage />} />
            <Route path="*" element={null} />
          </Routes>
          <Where />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return { calls, off };
}

afterEach(() => vi.unstubAllGlobals());

test("the list: online, last seen, revoked and warnings", async () => {
  setup();
  const card = async (id: string) => within(await screen.findByTestId(`host-${id}`));
  const pc = await card("h1");
  expect(pc.getByText("online")).toBeTruthy();
  expect(pc.getByText(/őr 0\.33\.0/)).toBeTruthy();
  expect(pc.getByText(/2 agent/)).toBeTruthy();
  expect(pc.getByRole("img", { name: /Claude-keret: 5 óra 42%/ })).toBeTruthy();
  const lab = await card("h2");
  expect(lab.getByText(/^utoljára 12 perccel ezelőtt$/)).toBeTruthy();
  expect(lab.getByText("1 elutasított mappa")).toBeTruthy();
  expect(lab.getByText("a csere-mappa nem írható")).toBeTruthy();
  const old = await card("h3");
  expect(old.getByText("visszavonva")).toBeTruthy();
  expect(old.getByText("még nem jelentkezett")).toBeTruthy();
});

test("overview shows the report", async () => {
  setup();
  expect(await screen.findByText("bwrap 0.9.0")).toBeTruthy();
  expect(screen.getByText("2.1.0")).toBeTruthy();
  expect(screen.getByText("működik")).toBeTruthy();
  expect(screen.getByText(/^5 óra: 42%/)).toBeTruthy();
  expect(screen.getByText(/^gemini-3\.8-flash-high: ⏸ .+-ig$/)).toBeTruthy();
  expect(screen.getByText(/^default: ⏸ elfogyott, újrapróbálás .+-kor$/)).toBeTruthy();
  expect(screen.getAllByText("⏸ agy ×2").length).toBeGreaterThan(0);
  expect(screen.getByText(/^Gemini Models: 5 óra 25% \(visszaáll .+\), heti 50% \(visszaáll –\)$/)).toBeTruthy();
});

test("agents tab: overview pane, settings params and the new-agent link", async () => {
  setup("/hosts/h1?c=1");
  fireEvent.mouseDown(await screen.findByRole("tab", { name: "Agentek" }));
  fireEvent.click(await screen.findByRole("button", { name: "@backend áttekintése" }));
  await waitFor(() => expect(screen.getByTestId("where").textContent).toContain("work=agent%3Aa1%3A"));
  fireEvent.click(screen.getByRole("link", { name: "@docs beállításai" }));
  await waitFor(() => {
    const q = new URLSearchParams(screen.getByTestId("where").textContent!.split("?")[1]);
    expect([q.get("agent"), q.get("settings"), q.get("c")]).toEqual(["docs", "profil", "1"]);
    expect(q.get("work")).toBe("agent:a1:");
  });
  fireEvent.click(screen.getByRole("button", { name: "Új agent ezen a gépen" }));
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/agents?new=1&host=h1"));
});

test("folders tab: who may read and write each granted folder", async () => {
  setup();
  fireEvent.mouseDown(await screen.findByRole("tab", { name: "Mappák" }));
  expect(await screen.findByRole("group", { name: "Írhatja: @backend" })).toBeTruthy();
  expect(screen.getByRole("group", { name: "Olvashatja: @docs" })).toBeTruthy();
  expect(screen.getByText("x")).toBeTruthy();
});

test("worktrees tab lists the reported worktrees", async () => {
  setup();
  fireEvent.mouseDown(await screen.findByRole("tab", { name: "Worktree-k és előnézetek" }));
  expect(await screen.findByText("colony/backend/work")).toBeTruthy();
  expect(await screen.findByText("Nincs előnézet ezen a gépen.")).toBeTruthy();
});

test("settings: a colour previews on the agents without their own colour, then PATCHes", async () => {
  const { calls } = setup();
  fireEvent.mouseDown(await screen.findByRole("tab", { name: "Beállítások" }));
  const preview = await screen.findByRole("list", { name: "Így látszanak az agentek" });
  const ring = (name: string) => (within(preview).getByRole("img", { name }) as HTMLElement).style.boxShadow;
  expect(ring("backend")).toContain("--color-hue-blue-solid");
  fireEvent.click(screen.getByRole("radio", { name: "narancs" }));
  expect(ring("backend")).toContain("--color-hue-orange-solid");
  expect(ring("docs")).toContain("--color-hue-pink-solid");
  fireEvent.click(screen.getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(calls).toContainEqual({ method: "PATCH", path: "/hosts/h1", body: { color: "orange" } }));
});

test("revoking asks first", async () => {
  const { calls } = setup();
  fireEvent.mouseDown(await screen.findByRole("tab", { name: "Beállítások" }));
  fireEvent.click(await screen.findByRole("button", { name: "Gép visszavonása" }));
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText("A gép minden agent-tokenje azonnal megszűnik.")).toBeTruthy();
  expect(calls.some((c) => c.method === "PUT")).toBe(false);
  fireEvent.click(within(dialog).getByRole("button", { name: "Visszavonás" }));
  await waitFor(() => expect(calls).toContainEqual({ method: "PUT", path: "/hosts/h1/state", body: { state: "revoked" } }));
});

test("the wizard shows the command and switches when the machine enrolls", async () => {
  let list = hosts();
  const { calls } = setup("/hosts", () => list);
  await screen.findByTestId("host-h1");
  fireEvent.click(screen.getByRole("button", { name: "Új gép" }));
  const dialog = await screen.findByRole("dialog");
  fireEvent.change(within(dialog).getByRole("textbox", { name: "A gép neve" }), { target: { value: "laptop" } });
  fireEvent.click(within(dialog).getByRole("button", { name: "Párosítókód kérése" }));
  expect(await within(dialog).findByText("K7Q2-MX4P")).toBeTruthy();
  expect(within(dialog).getByLabelText("Párosító parancs").textContent).toBe(`colony guard enroll -server ${window.location.origin} -code K7Q2-MX4P -name laptop`);
  expect(within(dialog).getByText(/^lejár: (10:00|9:59)$/)).toBeTruthy();
  expect(calls.some((c) => c.path === "/hosts/pairing-codes")).toBe(true);

  list = [...hosts(), { id: "h9", name: "laptop", color: "teal", state: "approved", created_at: ago(1), last_seen_at: ago(1), report: { roots: [], clients: [], rejected: [] }, agents: [] }];
  act(() => dispatch({ type: "event", event_type: "host.enrolled", seq: 1, actor_id: "me", at: "", data: { host_id: "h9", name: "laptop" } }));
  expect(await within(dialog).findByText("A(z) laptop gép csatlakozott.")).toBeTruthy();
  expect(within(dialog).getByLabelText("Telepítő parancs").textContent).toBe("colony guard install -config guard.yaml");
  fireEvent.click(within(dialog).getByRole("button", { name: "Kész" }));
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/hosts/h9"));
});

test("the overview shows the guard's health checks (round 41)", async () => {
  setup();
  expect(await screen.findByText("Claude Code: not logged in")).toBeTruthy();
  expect(screen.getByText("tmux")).toBeTruthy();
  expect(screen.getByText(/colony doctor/)).toBeTruthy();
});
