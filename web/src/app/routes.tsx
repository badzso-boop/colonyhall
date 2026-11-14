import { Route, Routes } from "react-router";
import { AuditPage, RunsPage, SettingsPage, StatusPage, TokensPage, UsersPage } from "@/features/admin";
import { AgentsPage } from "@/features/agents";
import { ConversationPage, DraftsPage, ThreadsPage } from "@/features/conversation";
import { DecisionsPage } from "@/features/decisions";
import { DesignPage } from "@/features/design";
import { HostsPage } from "@/features/hosts";
import { LoginPage } from "@/features/identity";
import { LeasesPage } from "@/features/leases";
import { NotesPage } from "@/features/notes";
import { RulesPage } from "@/features/rules";
import { SavedPage } from "@/features/saved";
import { SchedulesPage } from "@/features/schedules";
import { SearchPage } from "@/features/search";
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
        <Route path="threads" element={<ThreadsPage />} />
        <Route path="drafts" element={<DraftsPage />} />
        <Route path="saved" element={<SavedPage />} />
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
        <Route path="search" element={<SearchPage />} />
        <Route path="hosts" element={<HostsPage />} />
        <Route path="hosts/:hostId" element={<HostsPage />} />
        <Route path="admin/runs" element={<RunsPage />} />
        <Route path="admin/settings" element={<SettingsPage />} />
        <Route path="admin/users" element={<UsersPage />} />
        <Route path="admin/tokens" element={<TokensPage />} />
        <Route path="admin/audit" element={<AuditPage />} />
        <Route path="admin/status" element={<StatusPage />} />
        <Route path="*" element={<Home />} />
      </Route>
    </Routes>
  );
}
