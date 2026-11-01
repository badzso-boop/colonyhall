import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { identityKeys, type Actor } from "@/features/identity";
import { AccessButton } from "./AccessDialog";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "admin", state: "active", created_at: "" } as Actor;
const agent = (id: string, handle: string) => ({ ...me, id, kind: "agent", handle, display_name: handle, role: "member" }) as Actor;

function setup(viewer: Actor = me) {
  const puts: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      if (req.method === "PUT") {
        const body = await req.json();
        puts.push(body);
        return Response.json(body);
      }
      return Response.json({ task_all: true, task_agent_ids: [], cal_agent_ids: ["a2"] });
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.session, { actor: viewer, csrf_token: "x" });
  qc.setQueryData(identityKeys.actors, new Map([me, agent("a1", "trader-logs"), agent("a2", "personal-agy")].map((a) => [a.id, a])));
  render(
    <QueryClientProvider client={qc}>
      <AccessButton kind="tasks" />
      <AccessButton kind="calendar" />
    </QueryClientProvider>,
  );
  return puts;
}

afterEach(() => vi.unstubAllGlobals());

test("tasks: switching from every agent to selected ones sends the list and keeps the calendar", async () => {
  const puts = setup();
  fireEvent.click(screen.getAllByRole("button", { name: "Hozzáférés" })[0]!);
  const dialog = await screen.findByRole("dialog", { name: "Ki olvashatja a feladataidat?" });
  expect((await within(dialog).findByRole("radio", { name: "Minden agent" })).getAttribute("aria-checked")).toBe("true");
  fireEvent.click(within(dialog).getByRole("radio", { name: "Csak a kiválasztott agentek" }));
  fireEvent.click(within(dialog).getByRole("checkbox", { name: "@trader-logs" }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(puts).toEqual([{ task_all: false, task_agent_ids: ["a1"], cal_agent_ids: ["a2"] }]));
});

test("calendar: the chosen agents show checked; Senki clears them", async () => {
  const puts = setup();
  fireEvent.click(screen.getAllByRole("button", { name: "Hozzáférés" })[1]!);
  const dialog = await screen.findByRole("dialog", { name: "Ki olvashatja a naptáradat?" });
  expect((await within(dialog).findByRole("checkbox", { name: "@personal-agy" })).getAttribute("aria-checked")).toBe("true");
  fireEvent.click(within(dialog).getByRole("radio", { name: "Senki" }));
  fireEvent.click(within(dialog).getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(puts).toEqual([{ task_all: true, task_agent_ids: [], cal_agent_ids: [] }]));
});

test("an agent viewer gets no button", () => {
  setup(agent("a1", "trader-logs"));
  expect(screen.queryByRole("button", { name: "Hozzáférés" })).toBeNull();
});
