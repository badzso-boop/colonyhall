// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useMatch } from "react-router";
import { CircleHelp, Menu as MenuIcon, Search, ShieldAlert, Zap } from "lucide-react";
import { cn } from "@/shared/lib";
import { BrandMark, HoldButton, IconButton, Kbd, Tooltip } from "@/shared/ui";
import { EmergencyReleaseDialog, useEmergency, useEmergencyStop } from "@/features/admin";
import { useActivity } from "@/features/agents";
import { conversationTitle, useChannels } from "@/features/conversation";
import { useClaudeLimits } from "@/features/hosts";
import { useActors, useSession } from "@/features/identity";
import { QuestionsSheet, openQuestions, useOpenQuestions } from "@/features/questions";
import { openOverlay } from "../overlays";
import { WindowDock } from "../windows/WindowDock";
import { UserMenu } from "./UserMenu";

function Breadcrumb() {
  const match = useMatch("/c/:channelId");
  const channels = useChannels();
  const actors = useActors();
  const session = useSession();
  const ch = channels.data?.find((c) => c.id === match?.params.channelId);
  if (!ch || !actors.data) return null;
  const title = conversationTitle(ch, actors.data, session.data?.actor.id ?? "");
  return (
    <span title={title} className="hidden min-w-0 truncate text-ui text-fg-muted lg:block">
      {ch.kind === "dm" ? "" : "# "}
      <b className="font-medium text-fg">{title}</b>
    </span>
  );
}

function ClaudeMeter({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation("layout");
  const limits = useClaudeLimits(enabled);
  const five = limits.data?.five_hour;
  if (!five) return null;
  const pct = Math.round(five.utilization * 100);
  const week = Math.round((limits.data?.seven_day?.utilization ?? 0) * 100);
  const reset = five.resets_at ? new Date(five.resets_at).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" }) : "–";
  const label = t("claude", { five: pct, reset, week });
  return (
    <Tooltip content={label}>
      <span role="img" aria-label={label} className="grid size-8 place-items-center">
        <span
          className="grid size-5 place-items-center rounded-full"
          style={{ background: `conic-gradient(var(--color-${pct >= 80 ? "warning" : "fg-muted"}) ${pct}%, var(--color-border) 0)` }}
        >
          <span className="size-3 rounded-full bg-surface" />
        </span>
      </span>
    </Tooltip>
  );
}

/** The 48 px header on every page (round 31, part 4). */
export function Header({ onToggleSidebar }: { onToggleSidebar: () => void }) {
  const { t } = useTranslation("layout");
  const session = useSession();
  const me = session.data?.actor;
  const admin = me?.role === "owner" || me?.role === "admin";
  const questions = useOpenQuestions();
  const working = useActivity((s) => s.working);
  const workingCount = Object.keys(working).length;
  const emergency = useEmergency(admin);
  const stop = useEmergencyStop();
  const stopped = emergency.data?.active === true;
  const [release, setRelease] = useState(false);

  return (
    <>
      <header
        className={cn(
          "flex h-12 shrink-0 items-center gap-2 border-b px-3",
          stopped ? "border-accent bg-accent text-accent-fg" : "border-border bg-surface",
        )}
      >
        <IconButton label={t("toggleSidebar")} icon={<MenuIcon className="size-5" />} onClick={onToggleSidebar} className={stopped ? "text-accent-fg" : ""} />
        <Link to="/" className="flex items-center gap-2 text-title font-semibold whitespace-nowrap">
          <BrandMark className="size-6" />
          <span className="hidden sm:inline">{t("common:appName")}</span>
        </Link>
        <Breadcrumb />
        {stopped ? (
          <p className="mx-auto flex items-center gap-2 font-semibold">
            <ShieldAlert aria-hidden className="size-4" />
            {t("stopped")}
            {admin ? (
              <button type="button" onClick={() => setRelease(true)} className="rounded-md border border-accent-fg px-2 py-0.5 text-ui hover:bg-accent-hover">
                {t("release")}
              </button>
            ) : null}
          </p>
        ) : (
          <button
            type="button"
            onClick={() => openOverlay("palette")}
            className="mx-auto hidden h-8 min-w-0 flex-[0_1_26rem] items-center gap-2 rounded-md border border-border bg-surface-sunken px-2.5 text-ui text-fg-subtle hover:border-border-strong md:flex"
          >
            <Search aria-hidden className="size-4 shrink-0" />
            <span className="flex-1 truncate text-left">{t("search")}</span>
            <Kbd>Ctrl</Kbd>
            <Kbd>K</Kbd>
          </button>
        )}
        <div className="ml-auto flex items-center gap-1">
          <IconButton
            label={t("questions")}
            icon={<CircleHelp className="size-5" />}
            badge={questions.data?.length ?? 0}
            onClick={() => openQuestions()}
          />
          <Tooltip content={workingCount ? t("working", { count: workingCount }) : t("workingNone")}>
            <span className="hidden h-7 items-center gap-1.5 rounded-full border border-border px-2.5 text-meta text-fg-muted sm:inline-flex">
              <Zap aria-hidden className={cn("size-3.5", workingCount > 0 && "text-accent-text")} />
              <span className="tabular">{workingCount}</span>
            </span>
          </Tooltip>
          {admin ? <ClaudeMeter enabled={admin} /> : null}
          <WindowDock />
          {admin && !stopped ? (
            <HoldButton
              label={t("stop")}
              hint={t("stopHint")}
              icon={<ShieldAlert aria-hidden className="size-4" />}
              onConfirm={() => stop.mutate()}
              compact={false}
              className="hidden sm:inline-flex"
            />
          ) : null}
          <UserMenu />
        </div>
      </header>
      <QuestionsSheet />
      <EmergencyReleaseDialog open={release} onOpenChange={setRelease} />
      {me?.halted && !stopped ? (
        <div role="status" className="bg-warning-soft px-4 py-1 text-center text-meta text-warning">
          {t("halted")}
        </div>
      ) : null}
    </>
  );
}
