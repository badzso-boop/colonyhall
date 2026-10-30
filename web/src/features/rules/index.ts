import { lazy } from "react";

export { registerRulesEvents } from "./api";
export const RulesPage = lazy(() => import("./pages/RulesPage"));
