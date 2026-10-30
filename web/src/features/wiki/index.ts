import { lazy } from "react";

export { registerWikiEvents } from "./api";
export const WikiIndexPage = lazy(() => import("./pages/WikiIndexPage"));
export const WikiTopicPage = lazy(() => import("./pages/WikiTopicPage"));
