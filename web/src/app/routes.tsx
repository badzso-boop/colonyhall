import { Route, Routes } from "react-router";
import { DesignPage } from "@/features/design";
import { LoginPage } from "@/features/identity";
import { ScaffoldPage } from "@/features/scaffold";
import { RequireSession } from "./session";

// Route → the page a feature exports from its index.ts (lazy loaded).
// /login and /design are public; everything else needs a session.
export function AppRoutes() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
      <Route path="design" element={<DesignPage />} />
      <Route
        path="*"
        element={
          <RequireSession>
            <ScaffoldPage />
          </RequireSession>
        }
      />
    </Routes>
  );
}
