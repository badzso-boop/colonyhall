// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { identityKeys, useActors, useLogin, useLogout, useSession, useUpdateProfile, useVersion, type Actor, type BreakGlass, type Session } from "./api";
export { BreakGlassBanner } from "./components/BreakGlassBanner";
export { BreakGlassCard } from "./components/BreakGlassCard";
export { ProfileCard } from "./components/ProfileCard";
export { ProfileDialog } from "./components/ProfileDialog";
export { SourceLink } from "./components/SourceLink";
export { registerIdentityEvents } from "./events";
export { localTime, safeNext } from "./model";
export const LoginPage = lazy(() => import("./pages/LoginPage"));
export const InvitePage = lazy(() => import("./pages/InvitePage"));
export const BreakGlassPage = lazy(() => import("./pages/BreakGlassPage"));
