import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Outlet, useLocation } from "react-router";
import { cn } from "@/shared/lib";
import { useLocalStorage } from "@/shared/hooks";
import { useMentionTotal } from "@/features/conversation";
import { BottomBar } from "./BottomBar";
import { ConnectionBar } from "./ConnectionBar";
import { Header } from "./header/Header";
import { Sidebar } from "./sidebar/Sidebar";
import { useDocumentTitle } from "./useDocumentTitle";

/**
 * The app frame: header, sidebar, main column (docs/handoff/31, part 4).
 * ≥ 1024 px the sidebar is a column (collapsible); below it is a drawer;
 * on a phone the conversation list is the home screen and a bottom bar
 * switches screens.
 */
export function Shell() {
  const { t } = useTranslation("layout");
  const [collapsed, setCollapsed] = useLocalStorage("colony.sidebar.hidden", false);
  const [drawer, setDrawer] = useState(false);
  const location = useLocation();
  const home = location.pathname === "/";
  useDocumentTitle(useMentionTotal(), t("common:appName"));

  const toggle = () => {
    if (window.matchMedia("(min-width: 1024px)").matches) setCollapsed(!collapsed);
    else setDrawer(!drawer);
  };

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
            // Desktop: a column, unless collapsed.
            "lg:block lg:w-[260px]",
            collapsed && "lg:hidden",
            // Tablet: a drawer over the content.
            "max-lg:absolute max-lg:inset-y-0 max-lg:left-0 max-lg:z-(--z-overlay) max-lg:w-[280px] max-lg:shadow-overlay",
            drawer ? "max-lg:block" : "max-lg:hidden",
            // Phone: the list is the home screen.
            home && "max-md:static max-md:block max-md:w-full max-md:shadow-none",
          )}
          onClick={(e) => {
            if ((e.target as HTMLElement).closest("a")) setDrawer(false);
          }}
        >
          <Sidebar />
        </aside>
        {drawer ? <div aria-hidden className="absolute inset-0 z-(--z-pane) bg-overlay lg:hidden" onClick={() => setDrawer(false)} /> : null}
        <main id="main" className={cn("min-w-0 flex-1 bg-bg", home && "max-md:hidden")}>
          <Outlet />
        </main>
      </div>
      <BottomBar />
    </div>
  );
}
