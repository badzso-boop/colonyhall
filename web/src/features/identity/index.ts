import { lazy } from "react";

export { identityKeys, useActors, useLogin, useLogout, useSession, type Actor, type Session } from "./api";
export { registerIdentityEvents } from "./events";
export { safeNext } from "./model";
export const LoginPage = lazy(() => import("./pages/LoginPage"));
