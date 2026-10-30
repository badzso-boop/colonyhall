import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { ExternalLink } from "lucide-react";
import { cn } from "@/shared/lib";
import { openWindow } from "@/shared/windows";
import { ConversationList } from "@/features/conversation";
import { useSession } from "@/features/identity";
import { pages } from "./pages";

/** The sidebar: the workspace pages (with pop-out where a page floats), then the conversation groups. */
export function Sidebar({ ready }: { ready: Set<string> }) {
  const { t } = useTranslation("layout");
  const session = useSession();
  const role = session.data?.actor.role;
  const admin = role === "owner" || role === "admin";
  return (
    <div className="flex h-full flex-col overflow-y-auto px-2 pt-2 pb-6">
      <p className="px-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("workspace")}</p>
      <ul className="mt-1 flex flex-col gap-px">
        {pages
          .filter((p) => ready.has(p.id) && (!p.adminOnly || admin))
          .map((p) => {
            const Icon = p.icon;
            const name = t(`pages.${p.id}`);
            return (
              <li key={p.id} className="group relative">
                <NavLink
                  to={p.route}
                  className={({ isActive }) =>
                    cn(
                      "flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
                      isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
                    )
                  }
                >
                  <Icon aria-hidden className="size-4 text-fg-subtle" />
                  {name}
                </NavLink>
                {p.window ? (
                  <button
                    type="button"
                    aria-label={t("windows.popOut", { name })}
                    title={t("windows.popOut", { name })}
                    onClick={() => openWindow(p.window!)}
                    className="absolute top-1 right-1 grid size-6 place-items-center rounded-sm text-fg-subtle opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 hover:bg-surface hover:text-fg max-md:hidden"
                  >
                    <ExternalLink aria-hidden className="size-3.5" />
                  </button>
                ) : null}
              </li>
            );
          })}
      </ul>
      <ConversationList />
    </div>
  );
}
