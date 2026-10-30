import { Route, Routes } from "react-router";
import { ConversationPage } from "@/features/conversation";
import { DesignPage } from "@/features/design";
import { LoginPage } from "@/features/identity";
import { TasksPage } from "@/features/planner";
import { Shell } from "@/layout";
import { Home } from "./Home";
import { RequireSession } from "./session";

// Route → the page a feature exports from its index.ts (lazy loaded).
// /login and /design are public; everything else needs a session.
export function AppRoutes() {
  return (
    <Routes>
      <Route path="login" element={<LoginPage />} />
      <Route path="design" element={<DesignPage />} />
      <Route
        element={
          <RequireSession>
            <Shell />
          </RequireSession>
        }
      >
        <Route index element={<Home />} />
        <Route path="c/:channelId" element={<ConversationPage />} />
        <Route path="tasks" element={<TasksPage />} />
        <Route path="*" element={<Home />} />
      </Route>
    </Routes>
  );
}
