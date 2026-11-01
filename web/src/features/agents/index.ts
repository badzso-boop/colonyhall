import { lazy } from "react";

export { applyActivity, applyProgress, registerAgentEvents, stepKey, useActivity, type LiveStep, type WorkPlace } from "./activity";
export { agentKeys, useAgentChannel, useAgentOverview } from "./api";
export { AgentOverviewPane } from "./components/AgentOverviewPane";
export { AgentDialogs } from "./components/AgentDialogs";
export { ErrorChip } from "./components/ErrorChip";
export { StepDrawer, StepList } from "./components/StepDrawer";
export const AgentsPage = lazy(() => import("./pages/AgentsPage"));
