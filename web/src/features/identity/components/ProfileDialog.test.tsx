import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { identityKeys, type Actor, type Session } from "../api";
import { ProfileDialog } from "./ProfileDialog";

const me = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "admin", state: "active", created_at: "", timezone: "Europe/Budapest" } as Actor;

test("Profilom: the name is read-only; only changed fields are sent; the session follows", async () => {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string, init?: RequestInit) => {
      const req = input instanceof Request ? input : new Request(new URL(input, "http://localhost"), init);
      const body = await req.json();
      calls.push({ method: req.method, path: new URL(req.url).pathname, body });
      return Response.json({ ...me, ...body });
    }),
  );
  const qc = new QueryClient();
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "x" } as Session);
  qc.setQueryData(identityKeys.actors, new Map([["me", me]]));
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <ProfileDialog onClose={onClose} />
    </QueryClientProvider>,
  );
  const name = screen.getByRole("textbox", { name: /Megjelenített név/ }) as HTMLInputElement;
  expect(name.readOnly).toBe(true);
  expect(name.value).toBe("Norbi");
  fireEvent.change(screen.getByRole("textbox", { name: /Állapotüzenet/ }), { target: { value: " Úton " } });
  fireEvent.change(screen.getByRole("textbox", { name: /Avatar emoji/ }), { target: { value: "🦊" } });
  fireEvent.click(screen.getByRole("button", { name: "Mentés" }));
  await waitFor(() => expect(onClose).toHaveBeenCalled());
  expect(calls).toEqual([{ method: "PATCH", path: "/api/v1/actors/me/profile", body: { avatar_emoji: "🦊", status_text: "Úton" } }]);
  expect(qc.getQueryData<Session>(identityKeys.session)?.actor.status_text).toBe("Úton");
  expect(qc.getQueryData<Map<string, Actor>>(identityKeys.actors)?.get("me")?.avatar_emoji).toBe("🦊");
  vi.unstubAllGlobals();
});

test("too long a status keeps Mentés disabled", () => {
  const qc = new QueryClient();
  qc.setQueryData(identityKeys.session, { actor: me, csrf_token: "x" } as Session);
  render(
    <QueryClientProvider client={qc}>
      <ProfileDialog onClose={() => {}} />
    </QueryClientProvider>,
  );
  fireEvent.change(screen.getByRole("textbox", { name: /Állapotüzenet/ }), { target: { value: "x".repeat(81) } });
  expect((screen.getByRole("button", { name: "Mentés" }) as HTMLButtonElement).disabled).toBe(true);
});
