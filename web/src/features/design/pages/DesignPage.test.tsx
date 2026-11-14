// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@/shared/ui";
import DesignPage from "./DesignPage";

test("the style guide shows every section and opens its dialog", () => {
  render(
    <TooltipProvider>
      <DesignPage />
    </TooltipProvider>,
  );
  for (const name of ["Színek", "Tipográfia", "Gombok", "Mezők", "Jelvények", "Avatarok", "Ablakok", "Visszajelzés", "Logó"]) {
    expect(screen.getByRole("heading", { name })).toBeTruthy();
  }
  fireEvent.click(screen.getByRole("button", { name: "Dialógus" }));
  expect(screen.getByRole("dialog", { name: "Új csatorna" })).toBeTruthy();
});
