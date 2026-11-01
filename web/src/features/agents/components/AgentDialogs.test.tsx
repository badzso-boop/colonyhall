import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import AgentsPage from "../pages/AgentsPage";
import { AgentDialogs } from "./AgentDialogs";

const me = { id: "h", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const agent = (id: string, handle: string, extra = {}) => ({ id, kind: "agent", handle, display_name: handle, role: "member", state: "active", created_at: "", ...extra });
const config = { agent_id: "a1", handle: "backend", host_id: "pc", client: "claude-code", model: "sonnet", work_model: "sonnet", allow_work: true, permission_mode: "auto", mode: "direct",
  root_dir: "/home/n/projects/colonyhall", worktree_repo: "", sandbox: false, approvals: "none", max_session_tokens: 150000, daily_token_limit: 0, grants: [{ id: "g1", path: "/home/n/data", mode: "read" }] };
const hosts = [{ id: "pc", name: "pc-wsl", color: "teal", state: "approved", created_at: "", last_seen_at: new Date().toISOString(),
  report: { roots: ["/home/n"], clients: [{ name: "claude-code", version: "2.1" }], rejected: [] }, agents: [config] }];
const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });

function Loc() {
  const l = useLocation();
  return <output aria-label="location">{l.search}</output>;
}

function setup(url: string, ui: React.ReactNode = null, onPost?: (path: string, body: unknown) => Response | undefined) {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const u = new URL(req.url);
      const body = ["POST", "PATCH", "PUT"].includes(req.method) ? await req.json() : null;
      calls.push({ method: req.method, path: u.pathname, body });
      if (req.method !== "GET") return onPost?.(u.pathname, body) ?? (req.method === "DELETE" ? new Response(null, { status: 204 }) : json({ actor: agent("n1", "new-bot") }, 201));
      if (u.pathname.endsWith("/actors")) return json({ actors: [me, agent("a1", "backend", { description: "Go szerver", effective_color: "teal" }), agent("a2", "old", { archived_at: "2026-10-30T00:00:00Z", state: "stopped" })] });
      if (u.pathname.endsWith("/hosts")) return json({ hosts });
      if (u.pathname.endsWith("/channels")) return json({ channels: [{ id: "c1", kind: "public", name: "general", is_member: true, unread: 0, mentions: 0 }] });
      if (u.pathname.endsWith("/agent-rules")) return json({ version: 1, text: "You are @{handle}.", history: [] });
      if (u.pathname.endsWith("/instructions")) return json({ text: "Légy tömör.", effective: "You are @backend.\n\n## Instructions for @backend\n\nLégy tömör.", version: 1 });
      if (u.pathname.endsWith("/overview")) return json({ actor: agent("a1", "backend"), state: "active", usage: { today_tokens: 1234, last_runs: [] } });
      if (u.pathname.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["h", me]]));
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[url]}>
          {ui}
          <AgentDialogs />
          <Loc />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

test("creating: the warning on every step, the acknowledgement gates the button, one POST with everything", async () => {
  const calls = setup("/agents?new=1&host=pc");
  const dialog = await screen.findByRole("dialog");
  expect(within(dialog).getByText(/nem módosítható\. Változtatáshoz törölni kell/)).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("textbox", { name: /Handle/ }), { target: { value: "backend" } });
  expect(await within(dialog).findByText("Ezt a nevet egy aktív agent vagy ember használja.")).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("textbox", { name: /Handle/ }), { target: { value: "old" } });
  expect(within(dialog).getByText(/archivált agent neve volt/)).toBeTruthy();
  fireEvent.change(within(dialog).getByRole("textbox", { name: /Handle/ }), { target: { value: "new-bot" } });
  for (const step of ["Gép és kliens", "Hozzáférés", "Utasítás", "Keretek és csatornák", "Összegzés"]) {
    fireEvent.click(within(dialog).getByRole("button", { name: "Tovább" }));
    expect(within(dialog).getByRole("button", { name: new RegExp(step) }).getAttribute("aria-current")).toBe("step");
    expect(within(dialog).getByText(/nem módosítható\. Változtatáshoz törölni kell/)).toBeTruthy();
  }
  const create = within(dialog).getByRole("button", { name: "Létrehozás" }) as HTMLButtonElement;
  expect(create.disabled).toBe(true);
  fireEvent.click(within(dialog).getByRole("checkbox", { name: /Tudomásul veszem/ }));
  fireEvent.click(create);
  await waitFor(() => expect(calls.find((c) => c.method === "POST")).toBeTruthy());
  expect(calls.find((c) => c.method === "POST")!.body).toMatchObject({ handle: "new-bot", host: { host_id: "pc", client: "claude-code", mode: "compact", sandbox: true, permission_mode: "auto" } });
});

test("a server error jumps to the step of the field", async () => {
  setup("/agents?new=1&host=pc", null, () => json({ title: "Bad Request", status: 400, detail: "the path is outside the folders the machine reported" }, 400));
  const dialog = await screen.findByRole("dialog");
  fireEvent.change(within(dialog).getByRole("textbox", { name: /Handle/ }), { target: { value: "new-bot" } });
  fireEvent.click(within(dialog).getByRole("button", { name: /Összegzés/ }));
  fireEvent.click(within(dialog).getByRole("checkbox", { name: /Tudomásul veszem/ }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Létrehozás" }));
  await waitFor(() => expect(within(dialog).getByRole("button", { name: /Hozzáférés/ }).getAttribute("aria-current")).toBe("step"));
  expect(within(dialog).getByText("the path is outside the folders the machine reported")).toBeTruthy();
});

test("settings: the configuration is read-only with a copy; deleting needs the handle typed", async () => {
  const calls = setup("/agents?agent=backend&settings=konfiguracio");
  const dialog = await screen.findByRole("dialog");
  expect(await within(dialog).findByText("/home/n/projects/colonyhall")).toBeTruthy();
  expect(within(dialog).queryByRole("textbox")).toBeNull();
  fireEvent.click(within(dialog).getByRole("button", { name: /Utasítás/ }));
  expect((await within(dialog).findAllByText("Légy tömör.")).length).toBeGreaterThan(0);
  fireEvent.click(within(dialog).getByRole("button", { name: /Veszélyes/ }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Agent törlése" }));
  const confirm = (await screen.findAllByRole("dialog")).at(-1)!;
  const del = within(confirm).getByRole("button", { name: "Agent törlése" }) as HTMLButtonElement;
  expect(del.disabled).toBe(true);
  fireEvent.change(within(confirm).getByRole("textbox"), { target: { value: "backend" } });
  fireEvent.click(del);
  await waitFor(() => expect(calls.some((c) => c.method === "DELETE" && c.path.endsWith("/agents/a1"))).toBe(true));
});

test("settings → copy as a new agent opens the creation dialog prefilled", async () => {
  setup("/agents?agent=backend&settings=konfiguracio");
  const dialog = await screen.findByRole("dialog");
  fireEvent.click(await within(dialog).findByRole("button", { name: "Másolat új agentként" }));
  await waitFor(() => expect(screen.getByRole("status", { name: "location", hidden: true }).textContent).toBe("?new=1&copy=backend"));
  const create = await screen.findByRole("dialog", { name: "Új agent" });
  expect((within(create).getByRole("textbox", { name: /Handle/ }) as HTMLInputElement).value).toBe("backend");
  expect(within(create).getByText("Ezt a nevet egy aktív agent vagy ember használja.")).toBeTruthy();
});

test("the directory: fuzzy search with highlights, filters in the URL, archived only on request", async () => {
  setup("/agents", <AgentsPage />);
  expect(await screen.findByText("Go szerver")).toBeTruthy();
  expect(screen.queryByText("@old")).toBeNull();
  fireEvent.change(screen.getByRole("searchbox"), { target: { value: "szerver" } });
  await waitFor(() => expect(document.querySelector("mark")?.textContent).toBe("szerver"));
  fireEvent.click(screen.getByRole("button", { name: "archivált" }));
  await waitFor(() => expect(screen.getByRole("status", { name: "location" }).textContent).toContain("archived=1"));
  fireEvent.click(screen.getByRole("button", { name: /pc-wsl/ }));
  expect(screen.getByRole("status", { name: "location" }).textContent).toContain("host=pc");
});
