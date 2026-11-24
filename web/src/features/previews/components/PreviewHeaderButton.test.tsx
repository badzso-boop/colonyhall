// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router";
import { useComposerInsert } from "@/shared/lib";
import { previewKeys } from "../api";
import type { Preview } from "../model";
import { PreviewHeaderButton } from "./PreviewHeaderButton";

const running = {
  id: "p1", workspace_id: "w", agent_id: "ag", host_id: "h", channel_id: "c1", name: "shop", compose_path: "compose.yaml",
  state: "running", url: "http://100.64.0.5:41000", proxy_url: "http://colony.example.com:8081/api/v1/previews/p1/proxy/",
  expires_at: "", created_at: "", updated_at: "2026-11-23T10:00:00Z",
} as Preview;

function Where() {
  return <span data-testid="where">{useLocation().search}</span>;
}

function setup(previews: Preview[]) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ previews }), { headers: { "Content-Type": "application/json" } })));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  qc.setQueryData(previewKeys.all, previews);
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={["/c/c1?thread=t9"]}>
        <PreviewHeaderButton channelId="c1" />
        <Where />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.unstubAllGlobals());

test("with a live preview here the button opens the split view, keeping the panes", () => {
  setup([running]);
  fireEvent.click(screen.getByRole("button", { name: "Előnézet" }));
  const q = new URLSearchParams(screen.getByTestId("where").textContent!);
  expect(q.get("preview")).toBe("p1");
  expect(q.get("thread")).toBe("t9");
});

test("with a stopped one it restarts it and opens the new preview", async () => {
  setup([{ ...running, state: "stopped" }]);
  const fetchMock = vi.mocked(fetch);
  fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
    const req = input instanceof Request ? input : new Request(String(input));
    if (req.method === "POST" && req.url.endsWith("/previews/p1/restart")) {
      return new Response(JSON.stringify({ ...running, id: "p2", state: "requested" }), { status: 201, headers: { "Content-Type": "application/json" } });
    }
    return new Response(JSON.stringify({ previews: [] }), { headers: { "Content-Type": "application/json" } });
  });
  fireEvent.click(screen.getByRole("button", { name: "Újraindítás" }));
  await waitFor(() => expect(new URLSearchParams(screen.getByTestId("where").textContent!).get("preview")).toBe("p2"));
  expect(useComposerInsert.getState().text ?? "").not.toContain("preview_start");
});

test("with none here it asks the agent in the composer", () => {
  setup([{ ...running, channel_id: "other" }]);
  const before = useComposerInsert.getState().nonce;
  fireEvent.click(screen.getByRole("button", { name: "Előnézet indítása" }));
  expect(useComposerInsert.getState().nonce).toBe(before + 1);
  expect(useComposerInsert.getState().text).toContain("preview_start");
  expect(new URLSearchParams(screen.getByTestId("where").textContent!).get("preview")).toBeNull();
});
