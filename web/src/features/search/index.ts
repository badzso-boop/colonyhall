import { lazy } from "react";

export { searchPath } from "./model";
export const SearchPage = lazy(() => import("./pages/SearchPage"));
