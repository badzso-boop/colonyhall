import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { Palette } from "lucide-react";
import { cn } from "@/shared/lib";
import { ConversationList } from "@/features/conversation";

/** The sidebar: the workspace pages, then the conversation groups. */
export function Sidebar() {
  const { t } = useTranslation("layout");
  return (
    <div className="flex h-full flex-col overflow-y-auto px-2 pt-2 pb-6">
      <p className="px-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("workspace")}</p>
      <NavLink
        to="/design"
        className={({ isActive }) =>
          cn("mt-1 flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg", isActive && "bg-accent-soft text-fg")
        }
      >
        <Palette aria-hidden className="size-4 text-fg-subtle" />
        {t("pages.design")}
      </NavLink>
      <ConversationList />
    </div>
  );
}
