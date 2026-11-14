// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Inbox } from "lucide-react";
import { useNow } from "@/shared/hooks";
import { localeOf } from "@/shared/i18n";
import { relativeTime } from "@/shared/lib";
import { stepKey, useActivity, type WorkPlace } from "../activity";
import { formatDuration, placeHref, type AgentOverview } from "../model";
import { Section } from "./Section";

const noPlaces: WorkPlace[] = [];

/** 1. Most: where the agent works now, its live step and elapsed time, and its inbox. */
export function NowSection({ overview, channelName }: { overview: AgentOverview; channelName: (id: string) => string }) {
  const { t, i18n } = useTranslation("agents");
  const locale = localeOf(i18n.language);
  const agentId = overview.actor.id;
  const places = useActivity((s) => s.working[agentId] ?? noPlaces);
  const steps = useActivity((s) => s.steps);
  const now = useNow(1000);
  const inbox = overview.inbox;
  return (
    <Section id="now" title={t("overview.now.title")}>
      {places.length ? (
        <ul className="flex flex-col gap-2">
          {places.map((p) => {
            const step = steps[stepKey(agentId, p)];
            return (
              <li key={`${p.channelId}/${p.threadRootId ?? ""}`} className="flex flex-col gap-0.5">
                <Link to={placeHref(p.channelId, p.threadRootId)} className="flex items-center gap-1.5 text-accent-text hover:underline">
                  <span aria-hidden className="size-1.5 rounded-full bg-accent motion-safe:animate-pulse" />#{channelName(p.channelId)}
                  {p.threadRootId ? <span className="text-fg-muted">· {t("overview.now.thread")}</span> : null}
                </Link>
                {step ? (
                  <span className="tabular truncate pl-3 text-meta text-fg-muted">
                    {t("overview.now.step", { step: step.label, elapsed: formatDuration(now.getTime() - new Date(step.startedAt).getTime()) })}
                  </span>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="text-fg-muted">{t("overview.now.idle")}</p>
      )}
      {inbox ? (
        <p className="mt-2 flex items-center gap-1.5 text-fg-muted">
          <Inbox aria-hidden className="size-4 shrink-0" />
          {inbox.count > 0 ? (
            <span>
              {t("overview.now.inbox", { count: inbox.count })}
              {inbox.oldest_at ? ` · ${t("overview.now.inboxOldest", { age: relativeTime(inbox.oldest_at, now, locale) })}` : ""}
            </span>
          ) : (
            <span>{t("overview.now.inboxEmpty")}</span>
          )}
        </p>
      ) : null}
    </Section>
  );
}
