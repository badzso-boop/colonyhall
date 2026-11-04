import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { Schemas } from "@/shared/api";
import { identityKeys } from "@/features/identity";
import { TooltipProvider } from "@/shared/ui";
import { ClaudeLoginRow, OwnerRow, mayManageLogin } from "./HostAccess";

type Actor = Schemas["Actor"];
type Host = Schemas["Host"];

const person = (id: string, handle: string, role: Actor["role"]): Actor => ({ id, kind: "human", handle, display_name: handle, role, state: "active", created_at: "" });
const json = (b: unknown, status = 200) => new Response(status === 204 ? null : JSON.stringify(b), { status, headers: { "Content-Type": "application/json" } });
const problem = (status: number, detail: string) =>
  new Response(JSON.stringify({ type: "about:blank", title: "x", status, detail }), { status, headers: { "Content-Type": "application/problem+json" } });

const TOKEN = "sk-ant-oat01-secret-token-never-shown-again";

function host(extra: Partial<Host> = {}, report: Partial<Host["report"]> = {}): Host {
  return {
    id: "h1", name: "homelab-colleague", color: "blue", state: "approved", created_at: "", owner_id: "kollega",
    report: { roots: [], clients: [], rejected: [], claude_login_configured: true, ...report }, agents: [], ...extra,
  };
}

function setup(me: Actor, h: Host, answer: () => Response) {
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const p = new URL(req.url).pathname.replace(/^\/api\/v1/, "");
      if (p.endsWith("/auth/session")) return json({ actor: me, csrf_token: "x" });
      if (p === "/hosts") return json({ hosts: [h] });
      if (p.endsWith("/claude-login")) {
        bodies.push(await req.json().catch(() => null));
        return answer();
      }
      if (p.endsWith("/owner")) {
        bodies.push(await req.json().catch(() => null));
        return json(h);
      }
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.actors, new Map([["norbi", person("norbi", "norbi", "owner")], ["kollega", person("kollega", "kollega", "member")]]));
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <dl>
          <OwnerRow host={h} />
          <ClaudeLoginRow host={h} />
        </dl>
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return { bodies, qc };
}

afterEach(() => vi.unstubAllGlobals());

async function enter(token: string) {
  fireEvent.click(await screen.findByRole("button", { name: "Belépés beállítása" }));
  const field = await screen.findByLabelText("Token");
  expect(field.getAttribute("type")).toBe("password");
  fireEvent.change(field, { target: { value: token } });
  fireEvent.click(screen.getByRole("button", { name: "Mentés" }));
  return field as HTMLInputElement;
}

test("the machine's owner sets the login; the token is sent once and kept nowhere", async () => {
  const { bodies, qc } = setup(person("kollega", "kollega", "member"), host(), () => json({ set_at: "2026-11-04T12:00:00Z" }));
  expect(await screen.findByText("nincs")).toBeTruthy();
  const field = await enter(TOKEN);
  await waitFor(() => expect(bodies).toEqual([{ token: TOKEN }]));
  expect(field.value).toBe("");
  await waitFor(() => expect(screen.queryByLabelText("Token")).toBeNull());
  // Nothing in the query or mutation caches carries the token.
  const cached = JSON.stringify([qc.getQueryCache().getAll().map((q) => q.state.data), qc.getMutationCache().getAll().map((m) => m.state.variables)]);
  expect(cached).not.toContain("secret-token");
  expect(document.body.textContent).not.toContain("secret-token");
});

test("a refused token keeps the old login and says why", async () => {
  setup(person("kollega", "kollega", "member"), host({}, { claude_login_set_at: "2026-11-01T10:00:00Z" }), () => problem(422, "claude_login_check_failed"));
  const field = await enter(TOKEN);
  expect(await screen.findByText(/A Claude nem fogadta el a tokent/)).toBeTruthy();
  expect(field.value).toBe("");
  expect(document.body.textContent).not.toContain("secret-token");
});

test("an offline machine stores nothing", async () => {
  setup(person("kollega", "kollega", "member"), host(), () => problem(409, "host_offline"));
  await enter(TOKEN);
  expect(await screen.findByText(/A gép nem elérhető/)).toBeTruthy();
});

test("only the machine's owner and admins see the login; only the workspace owner changes the owner", async () => {
  expect(mayManageLogin(host(), { id: "kollega", role: "member" })).toBe(true);
  expect(mayManageLogin(host(), { id: "other", role: "member" })).toBe(false);
  expect(mayManageLogin(host({ owner_id: undefined }), { id: "x", role: "admin" })).toBe(true);

  setup(person("other", "other", "member"), host(), () => json({}));
  expect(await screen.findByText("@kollega")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Belépés beállítása" })).toBeNull();
  expect(screen.queryByRole("combobox", { name: "Tulajdonos" })).toBeNull();
});

test("the workspace owner gives the machine to a person", async () => {
  const { bodies } = setup(person("norbi", "norbi", "owner"), host({ owner_id: undefined }), () => json({}));
  const select = await screen.findByRole("combobox", { name: "Tulajdonos" });
  fireEvent.change(select, { target: { value: "kollega" } });
  await waitFor(() => expect(bodies).toEqual([{ owner_id: "kollega" }]));
});
