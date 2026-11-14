// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { stepKey, useActivity } from "@/features/agents";
import type { Actor } from "../model";

function elapsed(since: string, now: number): string {
  const s = Math.max(0, Math.floor((now - new Date(since).getTime()) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/** "@agent dolgozik… · <live step> · 1:04" above the composer of a channel or thread. */
export function ActivityLine({ channelId, threadRootId, actors }: { channelId: string; threadRootId?: string; actors: Map<string, Actor> }) {
  const { t } = useTranslation("conversation");
  const working = useActivity((s) => s.working);
  const steps = useActivity((s) => s.steps);
  const [now, setNow] = useState(() => Date.now());
  const here = Object.entries(working).filter(([, places]) =>
    places.some((p) => p.channelId === channelId && (threadRootId ? p.threadRootId === threadRootId : true)),
  );
  useEffect(() => {
    if (!here.length) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [here.length]);
  if (!here.length) return null;
  return (
    <div role="status" className="flex flex-col gap-0.5 px-4 pb-1 text-meta text-fg-muted">
      {here.map(([agentId]) => {
        const step = steps[stepKey(agentId, { channelId, ...(threadRootId ? { threadRootId } : {}) })] ?? steps[stepKey(agentId, { channelId })];
        return (
          <span key={agentId} className="flex min-w-0 items-center gap-2">
            <span aria-hidden className="size-2 shrink-0 rounded-full bg-accent motion-safe:animate-pulse" />
            <b className="font-mono font-medium text-fg">@{actors.get(agentId)?.handle ?? "?"}</b>
            {t("message.working")}
            {step ? <span className="truncate">· {step.label}</span> : null}
            {step ? <span className="tabular">· {elapsed(step.startedAt, now)}</span> : null}
          </span>
        );
      })}
    </div>
  );
}
