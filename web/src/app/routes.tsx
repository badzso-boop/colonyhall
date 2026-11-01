import { Route, Routes } from "react-router";
import { AgentsPage } from "@/features/agents";
import { ConversationPage } from "@/features/conversation";
import { DecisionsPage } from "@/features/decisions";
import { DesignPage } from "@/features/design";
import { LoginPage } from "@/features/identity";
import { LeasesPage } from "@/features/leases";
import { NotesPage } from "@/features/notes";
import { RulesPage } from "@/features/rules";
import { SchedulesPage } from "@/features/schedules";
import { WikiIndexPage, WikiTopicPage } from "@/features/wiki";
import { CalendarPage, TasksPage } from "@/features/planner";
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
        <Route path="agents" element={<AgentsPage />} />
        <Route path="tasks" element={<TasksPage />} />
        <Route path="calendar" element={<CalendarPage />} />
        <Route path="notes" element={<NotesPage />} />
        <Route path="schedules" element={<SchedulesPage />} />
        <Route path="wiki" element={<WikiIndexPage />} />
        <Route path="wiki/:slug" element={<WikiTopicPage />} />
        <Route path="leases" element={<LeasesPage />} />
        <Route path="rules" element={<RulesPage />} />
        <Route path="decisions" element={<DecisionsPage />} />
        <Route path="*" element={<Home />} />
      </Route>
    </Routes>
  );
}
