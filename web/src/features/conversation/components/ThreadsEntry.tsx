import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { MessagesSquare } from "lucide-react";
import { useSession } from "@/features/identity";
import { cn } from "@/shared/lib";
import { Badge } from "@/shared/ui";
import { useUnreadThreads } from "../api";

/** The sidebar's "Szálak" entry above the pages, with the number of threads that have unread replies. */
export function ThreadsEntry() {
  const { t } = useTranslation("conversation");
  const human = useSession().data?.actor.kind === "human";
  const unread = useUnreadThreads(human);
  if (!human) return null;
  return (
    <NavLink
      to="/threads"
      className={({ isActive }) =>
        cn(
          "relative flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
          unread > 0 && "font-semibold text-fg",
          isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
        )
      }
    >
      <MessagesSquare aria-hidden className="size-4 text-fg-subtle" />
      <span className="flex-1">{t("threads.title")}</span>
      {unread > 0 ? (
        <Badge tone="danger">
          <span className="sr-only">{t("threads.unreadThreads", { count: unread })}</span>
          <span aria-hidden>{unread}</span>
        </Badge>
      ) : null}
    </NavLink>
  );
}
