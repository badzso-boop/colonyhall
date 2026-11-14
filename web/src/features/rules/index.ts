// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerRulesEvents } from "./api";
export const RulesPage = lazy(() => import("./pages/RulesPage"));
