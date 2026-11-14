// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import type { Schemas } from "@/shared/api";
import { localeOf } from "@/shared/i18n";
import { hueVar } from "@/shared/lib";
import { Avatar, Badge, Tooltip, type AvatarSize } from "@/shared/ui";
import { agentColor, type GrantHolder } from "../model";

type Actor = Schemas["Actor"];

/** "14:05" in the interface language, or "–". */
export function clock(at: string | undefined, lang: string): string {
  return at ? new Date(at).toLocaleTimeString(localeOf(lang), { hour: "2-digit", minute: "2-digit" }) : "–";
}

/** "nov. 3. 14:05" in the interface language, or "–". */
export function stamp(at: string | undefined, lang: string): string {
  return at ? new Date(at).toLocaleString(localeOf(lang), { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }) : "–";
}

/** The Claude 5-hour window as a small ring (amber from 80 %), with both windows in the label. */
export function LimitRing({ limits }: { limits: Schemas["ClaudeLimits"] | undefined }) {
  const { t, i18n } = useTranslation("hosts");
  const five = limits?.five_hour;
  if (!five) return null;
  const pct = Math.round(five.utilization * 100);
  const week = Math.round((limits.seven_day?.utilization ?? 0) * 100);
  const label = t("limits.ring", { five: pct, reset: clock(five.resets_at, i18n.language), week });
  return (
    <Tooltip content={label}>
      <span role="img" aria-label={label} className="inline-flex items-center gap-1 text-meta text-fg-muted">
        <span
          className="grid size-4 place-items-center rounded-full"
          style={{ background: `conic-gradient(var(--color-${pct >= 80 ? "warning" : "fg-muted"}) ${pct}%, var(--color-border) 0)` }}
        >
          <span className="size-2.5 rounded-full bg-surface" />
        </span>
        <span className="tabular" aria-hidden>
          {pct}%
        </span>
      </span>
    </Tooltip>
  );
}

/** An agent's square avatar with its colour ring (its own, else the machine's). */
export function AgentAvatar({ actor, handle, hostColor, size = "md" }: { actor: Actor | undefined; handle: string; hostColor: string; size?: AvatarSize }) {
  return <Avatar kind="agent" size={size} name={actor?.display_name || handle} emoji={actor?.avatar_emoji} ringColor={hueVar(agentColor(actor, hostColor))} />;
}

/** The agents holding a grant on one folder: a read stack and a write stack. */
export function GrantStacks({ holders, actors, hostColor }: { holders: GrantHolder[]; actors: Map<string, Actor> | undefined; hostColor: string }) {
  const { t } = useTranslation("hosts");
  return (
    <span className="flex items-center gap-2">
      {(["read", "write"] as const).map((mode) => {
        const list = holders.filter((h) => h.mode === mode);
        if (!list.length) return null;
        const names = list.map((h) => `@${h.handle}`).join(", ");
        const label = t(`folders.${mode}By`, { names });
        return (
          <Tooltip key={mode} content={label}>
            <span role="group" aria-label={label} className="flex items-center gap-1">
              <Badge tone={mode === "write" ? "warning" : "neutral"}>{t(`folders.${mode}Short`)}</Badge>
              <span className="flex -space-x-1">
                {list.map((h) => (
                  <AgentAvatar key={h.agentId} actor={actors?.get(h.agentId)} handle={h.handle} hostColor={hostColor} size="sm" />
                ))}
              </span>
            </span>
          </Tooltip>
        );
      })}
    </span>
  );
}
