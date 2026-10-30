import { lazy } from "react";

export { registerLeaseEvents } from "./api";
export const LeasesPage = lazy(() => import("./pages/LeasesPage"));
