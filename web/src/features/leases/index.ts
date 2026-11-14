// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerLeaseEvents } from "./api";
export const LeasesPage = lazy(() => import("./pages/LeasesPage"));
