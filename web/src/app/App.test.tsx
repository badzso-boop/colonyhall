// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { App } from "./App";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const session = {
  actor: { id: "11111111-1111-1111-1111-111111111111", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "2026-10-30T00:00:00Z" },
  csrf_token: "csrf",
};

class NoopSocket {
  readyState = 0;
  onopen = null;
  onmessage = null;
  onclose = null;
  send() {}
  close() {}
}

beforeEach(() => vi.stubGlobal("WebSocket", NoopSocket));
afterEach(() => vi.unstubAllGlobals());

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  );
}

test("without a session, the app goes to /login and returns after signing in", async () => {
  let signedIn = false;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (path.endsWith("/auth/session")) return signedIn ? json(session) : json({ title: "Unauthorized" }, 401);
      if (path.endsWith("/actors")) return json({ actors: [session.actor] });
      if (path.endsWith("/channels")) return json({ channels: [] });
      if (path.endsWith("/sidebar/sections")) return json({ sections: [] });
      if (path.endsWith("/questions")) return json({ questions: [] });
      if (path.endsWith("/auth/login")) {
        const body = await req.json();
        if (body.handle !== "norbi" || body.password !== "jo") return json({ title: "Unauthorized" }, 401);
        signedIn = true;
        return json(session);
      }
      return json({});
    }),
  );
  renderAt("/c/abc");
  expect(await screen.findByRole("heading", { name: "Belépés" })).toBeTruthy();

  fireEvent.change(screen.getByLabelText("Felhasználónév"), { target: { value: "@Norbi" } });
  fireEvent.change(screen.getByLabelText("Jelszó"), { target: { value: "rossz" } });
  fireEvent.click(screen.getByRole("button", { name: "Belépés" }));
  expect(await screen.findByRole("alert")).toHaveProperty("textContent", "Hibás felhasználónév vagy jelszó.");

  fireEvent.change(screen.getByLabelText("Jelszó"), { target: { value: "jo" } });
  fireEvent.click(screen.getByRole("button", { name: "Belépés" }));
  await waitFor(() => expect(screen.queryByRole("heading", { name: "Belépés" })).toBeNull());
  // The shell: the user menu in the header and the home screen.
  expect(await screen.findByRole("button", { name: "Felhasználói menü" })).toBeTruthy();
  expect(await screen.findByText("Válassz egy beszélgetést")).toBeTruthy();
});

test("the style guide is public", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => json({ title: "Unauthorized" }, 401)));
  renderAt("/design");
  expect(await screen.findByRole("heading", { name: "Design system" })).toBeTruthy();
});

test("Ctrl+K opens the palette and ? the shortcut help with the registered shortcuts", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      if (path.endsWith("/auth/session")) return json(session);
      if (path.endsWith("/actors")) return json({ actors: [session.actor] });
      if (path.endsWith("/channels")) return json({ channels: [] });
      if (path.endsWith("/sidebar/sections")) return json({ sections: [] });
      if (path.endsWith("/questions")) return json({ questions: [] });
      return json({});
    }),
  );
  renderAt("/");
  await screen.findByRole("button", { name: "Felhasználói menü" });
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(await screen.findByRole("dialog", { name: "Parancspaletta" })).toBeTruthy();
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  fireEvent.keyDown(window, { key: "?", shiftKey: true });
  const help = await screen.findByRole("dialog", { name: "Billentyűparancsok" });
  expect(help.textContent).toContain("Parancspaletta megnyitása");
  expect(help.textContent).toContain("Következő olvasatlan beszélgetés");
});
