// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { registerAdminEvents, useEmergency, useEmergencyStop } from "./api";
export { EmergencyReleaseDialog } from "./components/EmergencyReleaseDialog";
export { DatabaseSizeCard } from "./components/DatabaseSizeCard";
export { EmergencyStopDialog } from "./components/EmergencyStopDialog";
export { SecretOnce } from "./components/SecretOnce";
export const RunsPage = lazy(() => import("./pages/RunsPage"));
export const SettingsPage = lazy(() => import("./pages/SettingsPage"));
export const UsersPage = lazy(() => import("./pages/UsersPage"));
export const TokensPage = lazy(() => import("./pages/TokensPage"));
export const AuditPage = lazy(() => import("./pages/AuditPage"));
export const StatusPage = lazy(() => import("./pages/StatusPage"));
