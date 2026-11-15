// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { TooltipProvider } from "@/shared/ui";
import { operatorKeys } from "../api";
import { OperatorShell } from "./OperatorShell";

function renderWith(devAccess: boolean) {
  const qc = new QueryClient();
  qc.setQueryData(operatorKeys.session, {
    operator: { id: "o", handle: "ops", display_name: "Ops" }, csrf_token: "c", expires_at: "2026-11-15T18:00:00Z", dev_access: devAccess, backup_codes_left: 10,
  });
  render(
    <QueryClientProvider client={qc}>
      <TooltipProvider>
        <MemoryRouter>
          <OperatorShell />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>,
  );
}

test("developer mode is announced on every page", () => {
  renderWith(true);
  expect(screen.getByRole("status").textContent).toContain("Fejlesztői mód: az üzemeltető minden adatot lát");
  expect(screen.getByRole("navigation", { name: "Üzemeltetés" })).toBeTruthy();
  expect(screen.getByText("@ops")).toBeTruthy();
});

test("no banner without developer mode", () => {
  renderWith(false);
  expect(screen.queryByRole("status")).toBeNull();
});
