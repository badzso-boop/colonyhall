// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerScheduleEvents } from "./api";
export { ScheduleBuilderHost } from "./components/ScheduleBuilderHost";
export const SchedulesPage = lazy(() => import("./pages/SchedulesPage"));
