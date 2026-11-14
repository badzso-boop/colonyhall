// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerWikiEvents } from "./api";
export const WikiIndexPage = lazy(() => import("./pages/WikiIndexPage"));
export const WikiTopicPage = lazy(() => import("./pages/WikiTopicPage"));
