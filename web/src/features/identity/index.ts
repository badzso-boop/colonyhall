import { lazy } from "react";

export { identityKeys, useActors, useLogin, useLogout, useSession, useUpdateProfile, useVersion, type Actor, type Session } from "./api";
export { ProfileCard } from "./components/ProfileCard";
export { ProfileDialog } from "./components/ProfileDialog";
export { SourceLink } from "./components/SourceLink";
export { registerIdentityEvents } from "./events";
export { localTime, safeNext } from "./model";
export const LoginPage = lazy(() => import("./pages/LoginPage"));
