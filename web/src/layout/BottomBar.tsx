// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { NavLink } from "react-router";
import { Bot, CircleHelp, MessagesSquare, MoreHorizontal } from "lucide-react";
import { cn } from "@/shared/lib";
import { openQuestions } from "@/features/questions";

/** The phone's bottom bar (< 768 px). Questions and agents get their pages in later rounds. */
export function BottomBar() {
  const { t } = useTranslation("layout");
  const item = "flex flex-1 flex-col items-center gap-0.5 py-1.5 text-meta text-fg-muted";
  return (
    <nav aria-label={t("tabs.conversations")} className="flex shrink-0 border-t border-border bg-surface pb-[env(safe-area-inset-bottom)] md:hidden">
      <NavLink to="/" end className={({ isActive }) => cn(item, isActive && "text-accent-text")}>
        <MessagesSquare aria-hidden className="size-5" />
        {t("tabs.conversations")}
      </NavLink>
      <button type="button" onClick={() => openQuestions()} className={item}>
        <CircleHelp aria-hidden className="size-5" />
        {t("tabs.questions")}
      </button>
      <a href="/" className={item}>
        <Bot aria-hidden className="size-5" />
        {t("tabs.agents")}
      </a>
      <a href="/" className={item}>
        <MoreHorizontal aria-hidden className="size-5" />
        {t("tabs.more")}
      </a>
    </nav>
  );
}
