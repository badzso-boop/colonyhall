// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { identityKeys } from "@/features/identity";
import { previewKeys } from "../api";
import { WIDTH_KEY, type Preview } from "../model";
import { LivePreviewPane, PreviewColumn } from "./LivePreviewPane";

const admin = { id: "me", kind: "human", handle: "norbi", display_name: "Norbi", role: "owner", state: "active", created_at: "" };
const agent = { id: "ag", kind: "agent", handle: "web-claude", display_name: "Web", role: "member", state: "active", created_at: "" };
const running: Preview = {
  id: "p1",
  workspace_id: "w",
  agent_id: "ag",
  host_id: "h",
  channel_id: "c1",
  name: "shop",
  compose_path: "compose.yaml",
  state: "running",
  url: "http://100.64.0.5:41000",
  proxy_url: "http://colony.example.com:8081/api/v1/previews/p1/proxy/",
  expires_at: "2026-11-23T12:00:00Z",
  created_at: "2026-11-23T10:00:00Z",
  updated_at: "2026-11-23T10:00:00Z",
};

function setup(ui: React.ReactNode, previews: Preview[] = [running]) {
  const calls: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (req: Request) => {
      const path = new URL(req.url).pathname;
      calls.push(`${req.method} ${path}`);
      const json = (b: unknown) => new Response(JSON.stringify(b), { status: 200, headers: { "Content-Type": "application/json" } });
      if (path.endsWith("/stop")) return json({ ...running, state: "stopped" });
      if (path.endsWith("/restart")) return json({ ...running, id: "p2", state: "requested" });
      if (path.endsWith("/previews")) return json({ previews });
      return json({});
    }),
  );
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(identityKeys.session, { actor: admin, csrf_token: "" });
  qc.setQueryData(identityKeys.actors, new Map([[admin.id, admin], [agent.id, agent]]));
  qc.setQueryData(previewKeys.all, previews);
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  localStorage.clear();
});

test("the frame shows the proxy address in a sandbox, with name, agent and state", () => {
  setup(<LivePreviewPane previewId="p1" onClose={() => {}} />);
  const frame = screen.getByTitle("shop előnézete") as HTMLIFrameElement;
  expect(frame.getAttribute("src")).toBe(running.proxy_url);
  expect(frame.getAttribute("sandbox")).toBe("allow-scripts allow-forms allow-same-origin allow-popups allow-modals");
  expect(screen.getByText("@web-claude")).toBeTruthy();
  expect(screen.getByText("fut")).toBeTruthy();
  const tab = screen.getByLabelText("Új lapon megnyitás");
  expect(tab.getAttribute("href")).toBe(running.proxy_url);
  expect(tab.getAttribute("target")).toBe("_blank");
  expect(tab.getAttribute("rel")).toContain("noopener");
});

test("refresh reloads the frame; close only closes the panel", () => {
  const onClose = vi.fn();
  const calls = setup(<LivePreviewPane previewId="p1" onClose={onClose} />);
  const before = screen.getByTitle("shop előnézete");
  fireEvent.click(screen.getByLabelText("Újratöltés"));
  expect(screen.getByTitle("shop előnézete")).not.toBe(before);
  fireEvent.click(screen.getByLabelText("Bezárás"));
  expect(onClose).toHaveBeenCalled();
  expect(calls.some((c) => c.endsWith("/stop"))).toBe(false);
});

test("stop asks first, then calls the stop endpoint", async () => {
  const calls = setup(<LivePreviewPane previewId="p1" onClose={() => {}} />);
  fireEvent.click(screen.getByLabelText("Leállítás"));
  expect(await screen.findByText("Leállítod a(z) shop előnézetet?")).toBeTruthy();
  expect(calls.some((c) => c.endsWith("/stop"))).toBe(false);
  const buttons = screen.getAllByRole("button", { name: "Leállítás" });
  fireEvent.click(buttons[buttons.length - 1]!);
  await waitFor(() => expect(calls).toContain("POST /api/v1/previews/p1/stop"));
});

test("a preview that is not running shows why instead of the frame", () => {
  setup(<LivePreviewPane previewId="p1" onClose={() => {}} />, [{ ...running, state: "requested" }]);
  expect(screen.queryByTitle("shop előnézete")).toBeNull();
  expect(screen.getByText("Az előnézet nem fut (indul). Amint elindul, itt megjelenik.")).toBeTruthy();
});

test("without a preview listener there is no frame", () => {
  setup(<LivePreviewPane previewId="p1" onClose={() => {}} />, [{ ...running, proxy_url: undefined }]);
  expect(screen.queryByTitle("shop előnézete")).toBeNull();
  expect(screen.getByText(/COLONY_PREVIEW_ADDR/)).toBeTruthy();
});

test("dragging the divider resizes the column within limits and keeps the width", () => {
  vi.stubGlobal("innerWidth", 1600);
  setup(<PreviewColumn previewId="p1" narrow={false} onClose={() => {}} />);
  const handle = screen.getByRole("separator", { name: "Az előnézet szélessége" });
  const column = handle.parentElement!;
  expect(column.style.width).toBe("500px");
  fireEvent.pointerDown(handle, { button: 0, clientX: 1000, pointerId: 1 });
  fireEvent.pointerMove(handle, { clientX: 900, pointerId: 1 });
  expect(column.style.width).toBe("600px");
  fireEvent.pointerUp(handle, { clientX: 100, pointerId: 1 });
  // 500 + 900 = 1400 > 80% of 1600.
  expect(column.style.width).toBe("1280px");
  expect(localStorage.getItem(WIDTH_KEY)).toBe("1280");
  fireEvent.keyDown(handle, { key: "ArrowRight", shiftKey: true });
  expect(column.style.width).toBe("1216px");
  expect(localStorage.getItem(WIDTH_KEY)).toBe("1216");
});

test("the stored width comes back, clamped to the window", () => {
  vi.stubGlobal("innerWidth", 1000);
  localStorage.setItem(WIDTH_KEY, "2000");
  setup(<PreviewColumn previewId="p1" narrow={false} onClose={() => {}} />);
  expect(screen.getByRole("separator").parentElement!.style.width).toBe("800px");
});

test("on a narrow screen the preview is a full-screen sheet without a divider", () => {
  setup(<PreviewColumn previewId="p1" narrow onClose={() => {}} />);
  expect(screen.getByRole("dialog", { name: "Élő előnézet" })).toBeTruthy();
  expect(screen.queryByRole("separator")).toBeNull();
});

test("a stopped preview restarts from the toolbar", async () => {
  const calls = setup(<LivePreviewPane previewId="p1" onClose={() => {}} />, [{ ...running, state: "stopped" }]);
  fireEvent.click(screen.getByRole("button", { name: "Újraindítás" }));
  await waitFor(() => expect(calls).toContain("POST /api/v1/previews/p1/restart"));
});

test("a running preview shows no restart button", () => {
  setup(<LivePreviewPane previewId="p1" onClose={() => {}} />);
  expect(screen.queryByRole("button", { name: "Újraindítás" })).toBeNull();
});
