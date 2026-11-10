import { Activity, Bot, CalendarDays, Clock, FileClock, Gauge, Gavel, KeyRound, KeySquare, ListChecks, NotebookPen, Palette, ScrollText, BookOpen, Search, Server, Settings, Users } from "lucide-react";
import type { WindowKind } from "@/shared/windows";

/** The workspace pages of the sidebar and the command palette (docs/handoff/32, part 2). */
export type PageEntry = { id: string; route: string; icon: typeof ListChecks; window?: WindowKind; adminOnly?: boolean; ownerOnly?: boolean };

export const pages: PageEntry[] = [
  { id: "search", route: "/search", icon: Search },
  { id: "agents", route: "/agents", icon: Bot },
  { id: "tasks", route: "/tasks", icon: ListChecks, window: "tasks" },
  { id: "calendar", route: "/calendar", icon: CalendarDays, window: "calendar" },
  { id: "notes", route: "/notes", icon: NotebookPen, window: "notes" },
  { id: "schedules", route: "/schedules", icon: Clock },
  { id: "wiki", route: "/wiki", icon: BookOpen },
  { id: "decisions", route: "/decisions", icon: Gavel },
  { id: "leases", route: "/leases", icon: KeyRound },
  { id: "rules", route: "/rules", icon: ScrollText, adminOnly: true },
  // Round 38: a person sees the machines they own (the server filters).
  { id: "hosts", route: "/hosts", icon: Server },
  { id: "design", route: "/design", icon: Palette },
];

/** The "Adminisztráció" group (docs/handoff/35, part 1). */
export const adminPages: PageEntry[] = [
  { id: "adminRuns", route: "/admin/runs", icon: Activity, adminOnly: true },
  { id: "adminSettings", route: "/admin/settings", icon: Settings, adminOnly: true },
  { id: "adminUsers", route: "/admin/users", icon: Users, adminOnly: true, ownerOnly: true },
  { id: "adminTokens", route: "/admin/tokens", icon: KeySquare, adminOnly: true },
  { id: "adminAudit", route: "/admin/audit", icon: FileClock, adminOnly: true },
  { id: "adminStatus", route: "/admin/status", icon: Gauge, adminOnly: true },
];
