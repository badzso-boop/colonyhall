// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

// The operator console (round 51): one lazily loaded chunk, so a workspace's
// app never loads it; App.tsx picks it on admin.<domain>.
export { isOperatorHost } from "./model";
export const OperatorApp = lazy(() => import("./pages/OperatorApp"));
