// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { i18n } from "./i18n";
import "@/shared/styles/index.css";
import { registerServiceWorker } from "@/shared/pwa";
import { toast } from "@/shared/ui";
import { App } from "./App";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");

if (import.meta.env.PROD) {
  registerServiceWorker((activate) =>
    toast(i18n.t("update.available"), { action: { label: i18n.t("update.reload"), run: activate }, ms: 24 * 60 * 60 * 1000 }),
  );
}

createRoot(root).render(
  <StrictMode>
    <BrowserRouter basename="/app">
      <App />
    </BrowserRouter>
  </StrictMode>,
);
