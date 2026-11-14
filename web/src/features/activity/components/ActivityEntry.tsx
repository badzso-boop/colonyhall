// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { Bell } from "lucide-react";
import { useSession } from "@/features/identity";
import { cn } from "@/shared/lib";
import { Badge } from "@/shared/ui";
import { useActivityUnread } from "../api";

/** The sidebar's "Tevékenység" entry with the unread count (round 55, part H). */
export function ActivityEntry() {
  const { t } = useTranslation("activity");
  const human = useSession().data?.actor.kind === "human";
  const unread = useActivityUnread(human) ?? 0;
  if (!human) return null;
  return (
    <NavLink
      to="/activity"
      className={({ isActive }) =>
        cn(
          "relative flex h-8 items-center gap-2 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg",
          unread > 0 && "font-semibold text-fg",
          isActive && "bg-accent-soft font-semibold text-fg before:absolute before:inset-y-1.5 before:-left-2 before:w-0.5 before:rounded-full before:bg-accent",
        )
      }
    >
      <Bell aria-hidden className="size-4 text-fg-subtle" />
      <span className="flex-1">{t("title")}</span>
      {unread > 0 ? (
        <Badge tone="danger">
          <span className="sr-only">{t("unreadCount", { count: unread })}</span>
          <span aria-hidden>{unread}</span>
        </Badge>
      ) : null}
    </NavLink>
  );
}
