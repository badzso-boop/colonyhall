import { useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { Outlet, useLocation } from "react-router";
import { ArrowLeft } from "lucide-react";
import { cn, useShortcut } from "@/shared/lib";
import { useLocalStorage, useWidth } from "@/shared/hooks";
import { place, usePanes } from "@/shared/panes";
import { ThreadPane, useConversationOrder, useMentionTotal } from "@/features/conversation";
import { AgentDialogs } from "@/features/agents";
import { ScheduleBuilderHost } from "@/features/schedules";
import { BottomBar } from "./BottomBar";
import { CommandPalette } from "./CommandPalette";
import { ConnectionBar } from "./ConnectionBar";
import { Header } from "./header/Header";
import { WorkPane } from "./panes/WorkPane";
import { openOverlay } from "./overlays";
import { ShortcutHelp } from "./ShortcutHelp";
import { Sidebar } from "./sidebar/Sidebar";
import { useDocumentTitle } from "./useDocumentTitle";
import { FloatingWindows } from "./windows/FloatingWindows";

/**
 * The app frame (docs/handoff/31, parts 4–5): header, sidebar, main column
 * and the two pane slots placed by shared/panes (place()).
 *  wide (≥ 1280): left slot = sidebar or the work pane, right slot = thread or work;
 *  medium: one right slot with a switcher; the sidebar is a column from 1024, a drawer below;
 *  narrow (< 768): one screen at a time with a back button, a bottom bar.
 */
export function Shell() {
  const { t } = useTranslation("layout");
  const width = useWidth();
  const [panes, dispatch] = usePanes();
  const placement = place(panes, width);
  const [collapsed, setCollapsed] = useLocalStorage("colony.sidebar.hidden", false);
  const [drawer, setDrawer] = useState(false);
  const location = useLocation();
  const home = location.pathname === "/" && placement.screen === "main";
  useDocumentTitle(useMentionTotal(), t("common:appName"));
  const navigate = useNavigate();
  const order = useConversationOrder();

  // Shortcuts of the frame (round 31 part 8); Radix closes dialogs on Esc itself.
  useShortcut("mod+k", "global", "palette", () => openOverlay("palette"));
  useShortcut("?", "global", "help", () => openOverlay("help"));
  const step = (dir: 1 | -1, unreadOnly: boolean) => {
    const current = order.findIndex((r) => location.pathname === `/c/${r.channel.id}`);
    for (let i = 1; i <= order.length; i++) {
      const r = order[(current + dir * i + order.length * 2) % order.length];
      if (r && (!unreadOnly || r.channel.unread > 0)) return navigate(`/c/${r.channel.id}${location.search}`);
    }
  };
  useShortcut("alt+arrowup", "global", "prevConversation", () => step(-1, false));
  useShortcut("alt+arrowdown", "global", "nextConversation", () => step(1, false));
  useShortcut("alt+shift+arrowup", "global", "prevUnread", () => step(-1, true));
  useShortcut("alt+shift+arrowdown", "global", "nextUnread", () => step(1, true));
  useShortcut("escape", "pane", "closePane", () => {
    if (panes.thread && (placement.right === "thread" || placement.screen === "thread")) dispatch({ type: "closeThread" });
    else if (panes.tabs.length) dispatch({ type: "closeWork" });
  });

  // Rule 6: while the work pane holds the left slot, the sidebar is a drawer.
  const sidebarAsDrawer = placement.left === "work";
  const toggle = () => {
    if (!sidebarAsDrawer && window.matchMedia("(min-width: 1024px)").matches) setCollapsed(!collapsed);
    else setDrawer(!drawer);
  };

  const thread = panes.thread ? <ThreadPane key={panes.thread} rootId={panes.thread} /> : null;
  const right = placement.right === "thread" ? thread : placement.right === "work" ? <WorkPane /> : null;
  const switcher =
    placement.other !== null ? (
      <button
        type="button"
        onClick={() => dispatch({ type: "focus", pane: placement.other! })}
        className="w-full border-b border-border bg-surface-sunken py-1 text-meta font-medium text-fg-muted hover:text-fg"
      >
        {placement.other === "thread" ? t("panes.showThread") : t("panes.showWork")}
      </button>
    ) : null;

  return (
    <div className="flex h-full flex-col">
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-(--z-toast) focus:rounded-md focus:bg-surface focus:px-3 focus:py-2">
        {t("skip")}
      </a>
      <Header onToggleSidebar={toggle} />
      <ConnectionBar />
      <div className="relative flex min-h-0 flex-1">
        <aside
          aria-label={t("sidebar")}
          className={cn(
            "shrink-0 border-r border-border bg-surface",
            sidebarAsDrawer && "absolute inset-y-0 left-0 z-(--z-overlay) w-[280px] shadow-overlay",
            !sidebarAsDrawer && "lg:block lg:w-[260px]",
            !sidebarAsDrawer && collapsed && "lg:hidden",
            !sidebarAsDrawer && "max-lg:absolute max-lg:inset-y-0 max-lg:left-0 max-lg:z-(--z-overlay) max-lg:w-[280px] max-lg:shadow-overlay",
            drawer ? "block" : sidebarAsDrawer ? "hidden" : "max-lg:hidden",
            home && "max-md:static max-md:block max-md:w-full max-md:shadow-none",
          )}
          onClick={(e) => {
            if ((e.target as HTMLElement).closest("a")) setDrawer(false);
          }}
        >
          <Sidebar />
        </aside>
        {drawer ? <div aria-hidden className="absolute inset-0 z-(--z-pane) bg-overlay" onClick={() => setDrawer(false)} /> : null}
        {placement.left === "work" ? (
          <div className="w-[560px] shrink-0 border-r border-border">
            <WorkPane />
          </div>
        ) : null}
        {placement.screen === "main" ? (
          <main id="main" tabIndex={-1} className={cn("min-w-0 flex-1 bg-bg outline-none", home && "max-md:hidden")}>
            <Outlet />
          </main>
        ) : (
          <main id="main" className="flex min-w-0 flex-1 flex-col bg-surface">
            <button
              type="button"
              onClick={() => window.history.back()}
              className="flex h-10 shrink-0 items-center gap-1.5 border-b border-border px-3 text-ui text-fg-muted"
            >
              <ArrowLeft aria-hidden className="size-4" />
              {t("panes.back")}
            </button>
            {switcher}
            <div className="min-h-0 flex-1">{placement.screen === "thread" ? thread : <WorkPane />}</div>
          </main>
        )}
        {right ? (
          <div className={cn("flex shrink-0 flex-col border-l border-border bg-surface", width === "wide" ? "w-[440px]" : "w-[380px]")}>
            {width === "medium" ? switcher : null}
            <div className="min-h-0 flex-1">{right}</div>
          </div>
        ) : null}
      </div>
      {placement.screen === "main" ? <BottomBar /> : null}
      <FloatingWindows />
      <ScheduleBuilderHost />
      <AgentDialogs />
      <CommandPalette />
      <ShortcutHelp />
    </div>
  );
}
