// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { FolderOpen, MessageSquare, MoreHorizontal, Octagon, Pause, Play, Settings, ShieldAlert, ShieldCheck, Square, SquareTerminal } from "lucide-react";
import { cn, hueVar } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Avatar, Badge, Button, Dialog, IconButton, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, toast } from "@/shared/ui";
import { useOpenDm, useSetAgentState } from "../api";
import type { AgentOverview, DisplayState } from "../model";
import { useOpenSettings } from "../settingsLink";

const stateTone = { working: "accent", idle: "neutral", offline: "neutral", paused: "warning", stopped: "danger", archived: "neutral" } as const;
const avatarStatus = { working: "working", idle: "idle", offline: "offline", paused: "paused", stopped: "offline", archived: "offline" } as const;

type Confirm = "stop" | "emergency" | null;

/** The overview header: identity, the state/host/client/access chips and the actions. */
export function OverviewHeader({ overview, state, hostColor, admin }: { overview: AgentOverview; state: DisplayState; hostColor?: string | undefined; admin: boolean }) {
  const { t } = useTranslation("agents");
  const { actor, assignment } = overview;
  const [, dispatch] = usePanes();
  const navigate = useNavigate();
  const openSettings = useOpenSettings(actor.handle);
  const dm = useOpenDm();
  const setState = useSetAgentState(actor.id);
  const [confirm, setConfirm] = useState<Confirm>(null);
  const archived = state === "archived";

  const change = (next: "active" | "paused" | "stopped") =>
    setState.mutate(next, {
      onSuccess: () => toast(t("overview.actions.done")),
      onError: () => toast(t("overview.actions.failed")),
      onSettled: () => setConfirm(null),
    });

  return (
    <header className="flex flex-col gap-3 border-b border-border p-4 sm:flex-row sm:items-start">
      <Avatar kind="agent" name={actor.handle} emoji={actor.avatar_emoji} size="lg" status={avatarStatus[state]} ringColor={hueVar(actor.effective_color)} className="size-14 rounded-lg text-page" />
      <div className="min-w-0 flex-1">
        <p className="text-title font-semibold">{actor.display_name}</p>
        <p className="font-mono text-meta text-fg-muted">@{actor.handle}</p>
        {actor.description ? <p className="mt-1 text-ui text-fg-muted">{actor.description}</p> : null}
        <div className="mt-2 flex flex-wrap gap-1.5">
          <Badge tone={stateTone[state]}>
            {state === "working" ? <span aria-hidden className="size-1.5 rounded-full bg-accent-fg motion-safe:animate-pulse" /> : null}
            {t(`overview.states.${state}`)}
          </Badge>
          {assignment ? (
            <Badge>
              <span aria-hidden className={cn("size-2 rounded-full", !hostColor && "bg-fg-subtle")} style={hostColor ? { backgroundColor: hostColor } : undefined} />
              <span title={t("overview.hostLabel", { host: assignment.host_name ?? assignment.host_id })}>{assignment.host_name ?? assignment.host_id}</span>
            </Badge>
          ) : null}
          {assignment ? <Badge>{`${assignment.client} · ${assignment.model}`}</Badge> : null}
          {assignment ? (
            assignment.sandbox ? (
              <Badge tone="info">
                <ShieldCheck aria-hidden className="size-3" />
                {t("overview.sandbox")}
              </Badge>
            ) : (
              <Badge tone="warning">
                <ShieldAlert aria-hidden className="size-3" />
                {t("overview.fullAccess")}
              </Badge>
            )
          ) : null}
        </div>
        <div role="group" aria-label={t("overview.actions.label")} className="mt-3 flex flex-wrap items-center gap-1.5">
          {!archived ? (
            <Button
              size="sm"
              icon={<MessageSquare className="size-3.5" />}
              loading={dm.isPending}
              onClick={() =>
                dm.mutate(actor.id, {
                  onSuccess: (ch) => void navigate(`/c/${ch.id}`),
                  onError: () => toast(t("overview.actions.dmFailed")),
                })
              }
            >
              {t("overview.actions.dm")}
            </Button>
          ) : null}
          <Button size="sm" icon={<FolderOpen className="size-3.5" />} onClick={() => dispatch({ type: "openWork", tab: { kind: "files", agentId: actor.id, ref: "" } })}>
            {t("overview.actions.files")}
          </Button>
          <Button size="sm" icon={<SquareTerminal className="size-3.5" />} onClick={() => dispatch({ type: "openWork", tab: { kind: "terminal", agentId: actor.id, ref: "" } })}>
            {t("overview.actions.terminals")}
          </Button>
          <Button size="sm" icon={<Settings className="size-3.5" />} onClick={() => openSettings("profil")}>
            {t("overview.actions.settings")}
          </Button>
          {admin && !archived ? (
            <Menu>
              <MenuTrigger asChild>
                <IconButton size="sm" label={t("overview.actions.more")} icon={<MoreHorizontal className="size-4" />} />
              </MenuTrigger>
              <MenuContent>
                {overview.state === "active" ? (
                  <MenuItem icon={<Pause className="size-4" />} onSelect={() => change("paused")}>
                    {t("overview.actions.pause")}
                  </MenuItem>
                ) : (
                  <MenuItem icon={<Play className="size-4" />} onSelect={() => change("active")}>
                    {t("overview.actions.resume")}
                  </MenuItem>
                )}
                {overview.state !== "stopped" ? (
                  <>
                    <MenuItem icon={<Square className="size-4" />} onSelect={() => setConfirm("stop")}>
                      {t("overview.actions.stop")}
                    </MenuItem>
                    <MenuSeparator />
                    <MenuItem danger icon={<Octagon className="size-4" />} onSelect={() => setConfirm("emergency")}>
                      {t("overview.actions.emergency")}
                    </MenuItem>
                  </>
                ) : null}
              </MenuContent>
            </Menu>
          ) : null}
        </div>
      </div>
      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm === "emergency" ? t("overview.actions.emergencyTitle", { handle: actor.handle }) : t("overview.actions.stopTitle", { handle: actor.handle })}
        description={confirm === "emergency" ? t("overview.actions.emergencyBody") : t("overview.actions.stopBody")}
        closeLabel={t("overview.actions.cancel")}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>{t("overview.actions.cancel")}</Button>
            {/* There is no per-agent emergency stop endpoint (POST /admin/emergency-stop
                halts the whole workspace), so "this one only" maps to the stopped state. */}
            <Button variant="danger" loading={setState.isPending} onClick={() => change("stopped")}>
              {confirm === "emergency" ? t("overview.actions.emergency") : t("overview.actions.stop")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </header>
  );
}
