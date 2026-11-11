import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import { TerminalPane } from "./TerminalPane";

const admin = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const agent = { id: "ag", kind: "agent", handle: "backend-claude", display_name: "Backend", role: "member", state: "active", created_at: "" };
const terms = [
  { name: "build", status: "running", command: "go test ./...", cwd: "/", channel_id: "c1", started_at: new Date().toISOString(), duration_ms: 64_000 },
  { name: "lint", status: "finished", command: "make lint", cwd: "/", channel_id: "c1", started_at: new Date().toISOString(), duration_ms: 3_000, exit_code: 2 },
];

function setup(name: string, relay: (body: { op: string; name?: string }) => unknown) {
  const ops: Array<{ op: string; name?: string }> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      const json = (b: unknown, status = 200) => new Response(JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
      if (path.endsWith("/overview")) return json({ actor: agent, state: "active", assignment: { host_name: "pc-dev" }, common_channels: [{ id: "c1", name: "general", kind: "public" }] });
      if (path.endsWith("/fs")) {
        const body = await req.json();
        ops.push(body);
        const r = relay(body);
        return r instanceof Response ? r : json(r);
      }
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.session, { actor: admin, csrf_token: "" });
  qc.setQueryData(identityKeys.actors, new Map([[admin.id, admin], [agent.id, agent]]));
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TerminalPane agentId="ag" name={name} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return ops;
}

afterEach(() => vi.unstubAllGlobals());

test("the list shows each terminal with its status", async () => {
  setup("", () => ({ op: "term_list", terminals: terms }));
  expect(await screen.findByText("build")).toBeTruthy();
  expect(screen.getByText("fut ⏳")).toBeTruthy();
  expect(screen.getByText("hiba ⚠️ exit 2")).toBeTruthy();
  expect(screen.getByText("1:04")).toBeTruthy();
});

test("the output is text, and an admin can stop a running terminal after confirming", async () => {
  const ops = setup("build", (b) =>
    b.op === "term_list" ? { terminals: terms } : b.op === "term_output" ? { output: "ok\n<b>not html</b>\n" } : { killed: true },
  );
  const pre = await screen.findByText(/not html/);
  expect(pre.querySelector("b")).toBeNull();
  expect(screen.getByText(/tmux -L colonyhall attach -r -t colony-backend-claude-build/)).toBeTruthy();
  fireEvent.click(await screen.findByRole("button", { name: "Leállítás" }));
  await screen.findByRole("dialog", { name: "Leállítod a(z) build terminált?" });
  const stops = screen.getAllByRole("button", { name: "Leállítás" });
  fireEvent.click(stops[stops.length - 1]!);
  await waitFor(() => expect(ops.some((o) => o.op === "term_kill" && o.name === "build")).toBe(true));
});

test("a machine that is offline gives a readable message", async () => {
  setup("", () => ({ op: "term_list", error_code: "host_offline" }));
  expect(await screen.findByText("A gép most nem elérhető.")).toBeTruthy();
});
