// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { Suspense, useEffect, useState } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { createQueryClient } from "@/shared/api";
import { installShortcuts } from "@/shared/lib";
import { Toaster, TooltipProvider } from "@/shared/ui";
import { AppRoutes } from "./routes";

export function App() {
  const [queryClient] = useState(createQueryClient);
  const { t } = useTranslation();
  useEffect(installShortcuts, []);
  return (
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <Suspense fallback={null}>
          <AppRoutes />
        </Suspense>
        <Toaster closeLabel={t("close")} />
      </TooltipProvider>
    </QueryClientProvider>
  );
}
