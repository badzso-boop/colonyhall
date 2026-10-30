import { lazy } from "react";

export { registerScheduleEvents } from "./api";
export { ScheduleBuilderHost } from "./components/ScheduleBuilderHost";
export const SchedulesPage = lazy(() => import("./pages/SchedulesPage"));
