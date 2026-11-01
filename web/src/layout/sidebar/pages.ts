import { Bot, CalendarDays, Clock, Gavel, KeyRound, ListChecks, NotebookPen, Palette, ScrollText, BookOpen, Server } from "lucide-react";
import type { WindowKind } from "@/shared/windows";

/** The workspace pages of the sidebar and the command palette (docs/handoff/32, part 2). */
export type PageEntry = { id: string; route: string; icon: typeof ListChecks; window?: WindowKind; adminOnly?: boolean };

export const pages: PageEntry[] = [
  { id: "agents", route: "/agents", icon: Bot },
  { id: "tasks", route: "/tasks", icon: ListChecks, window: "tasks" },
  { id: "calendar", route: "/calendar", icon: CalendarDays, window: "calendar" },
  { id: "notes", route: "/notes", icon: NotebookPen, window: "notes" },
  { id: "schedules", route: "/schedules", icon: Clock },
  { id: "wiki", route: "/wiki", icon: BookOpen },
  { id: "decisions", route: "/decisions", icon: Gavel },
  { id: "leases", route: "/leases", icon: KeyRound },
  { id: "rules", route: "/rules", icon: ScrollText, adminOnly: true },
  { id: "hosts", route: "/hosts", icon: Server, adminOnly: true },
  { id: "design", route: "/design", icon: Palette },
];
