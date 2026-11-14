// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerPlannerEvents } from "./api";
export { useWindowBadge } from "./badges";
export { CalendarWindow } from "./components/CalendarWindow";
export { TasksWindow } from "./components/TasksWindow";
export const TasksPage = lazy(() => import("./pages/TasksPage"));
export const CalendarPage = lazy(() => import("./pages/CalendarPage"));
