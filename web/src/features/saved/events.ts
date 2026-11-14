// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import type { QueryClient } from "@tanstack/react-query";
import { onEvent, onResync } from "@/shared/api";
import { savedKeys } from "./api";

/** The viewer's other tabs and devices saved, changed or were reminded of something. */
export function registerSavedEvents(qc: QueryClient): () => void {
  const refresh = () => void qc.invalidateQueries({ queryKey: savedKeys.all });
  const offs = [onEvent("saved.changed", refresh), onEvent("saved.reminded", refresh), onResync(refresh)];
  return () => offs.forEach((off) => off());
}
