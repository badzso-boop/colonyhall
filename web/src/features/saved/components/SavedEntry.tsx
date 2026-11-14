// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { Bookmark } from "lucide-react";
import { useSession } from "@/features/identity";
import { cn } from "@/shared/lib";

/** The sidebar's "Mentett" entry (round 55, part E). */
export function SavedEntry() {
  const { t } = useTranslation("saved");
  const human = useSession().data?.actor.kind === "human";
  if (!human) return null;
  return (
    <NavLink
      to="/saved"
      className={({ isActive }) =>
        cn(
          "relative flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
          isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
        )
      }
    >
      <Bookmark aria-hidden className="size-4 text-fg-subtle" />
      <span className="flex-1">{t("title")}</span>
    </NavLink>
  );
}
