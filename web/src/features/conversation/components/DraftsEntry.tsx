import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { PencilLine } from "lucide-react";
import { useSession } from "@/features/identity";
import { cn } from "@/shared/lib";
import { useDrafts } from "../api";

/** The sidebar's "Piszkozatok" entry (round 55, part A): shown while there is at least one draft. */
export function DraftsEntry() {
  const { t } = useTranslation("conversation");
  const human = useSession().data?.actor.kind === "human";
  const drafts = useDrafts(human);
  const count = drafts.data?.length ?? 0;
  if (!human || count === 0) return null;
  return (
    <NavLink
      to="/drafts"
      className={({ isActive }) =>
        cn(
          "relative flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
          isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
        )
      }
    >
      <PencilLine aria-hidden className="size-4 text-fg-subtle" />
      <span className="flex-1">{t("drafts.title")}</span>
      <span className="text-meta text-fg-subtle tabular">
        <span className="sr-only">{t("drafts.count", { count })}</span>
        <span aria-hidden>{count}</span>
      </span>
    </NavLink>
  );
}
