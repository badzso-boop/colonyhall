// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerActivityEvents } from "./events";
export { activityKeys, useActivityUnread, useMarkActivityRead } from "./api";
export { ActivityEntry } from "./components/ActivityEntry";
export const ActivityPage = lazy(() => import("./pages/ActivityPage"));
