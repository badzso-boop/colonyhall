import { lazy } from "react";

export { registerPlannerEvents } from "./api";
export { useWindowBadge } from "./badges";
export { CalendarWindow } from "./components/CalendarWindow";
export { TasksWindow } from "./components/TasksWindow";
export const TasksPage = lazy(() => import("./pages/TasksPage"));
