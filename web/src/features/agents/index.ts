// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { lazy } from "react";

export { applyActivity, applyProgress, registerAgentEvents, stepKey, useActivity, type LiveStep, type WorkPlace } from "./activity";
export { agentKeys, useAgentChannel, useAgentOverview } from "./api";
export { AgentOverviewPane } from "./components/AgentOverviewPane";
export { AgentDialogs } from "./components/AgentDialogs";
export { ErrorChip } from "./components/ErrorChip";
export { StepDrawer, StepList } from "./components/StepDrawer";
export { AgentProfileDetails } from "./components/AgentProfileDetails";
export const AgentsPage = lazy(() => import("./pages/AgentsPage"));
