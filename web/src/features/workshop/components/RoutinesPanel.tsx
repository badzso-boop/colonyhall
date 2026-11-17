// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { fullTime } from "@/shared/lib";
import { Badge, Button, Notice, Skeleton, toast } from "@/shared/ui";
import { useCreateWebhook, useRoutines } from "../api";

const runTone = {
  pending: "neutral",
  running: "neutral",
  done: "success",
  failed: "danger",
} as const;

/** An extension's live routines, their schedule, webhook addresses and the latest runs (round 56c). */
export function RoutinesPanel({ id }: { id: string }) {
  const { t, i18n } = useTranslation("workshop");
  const list = useRoutines(id);
  const webhook = useCreateWebhook(id);
  const [address, setAddress] = useState<{
    routine: string;
    url: string;
  } | null>(null);
  if (list.isPending) return <Skeleton className="h-12 w-full" />;
  if (list.isError)
    return <p className="text-ui text-danger">{list.error.message}</p>;
  const { routines, runs } = list.data;
  if (!routines.length && !runs.length) return null;
  return (
    <details>
      <summary className="cursor-pointer text-ui font-medium">
        {t("routines.title", { count: routines.length })}
      </summary>
      <ul className="mt-2 flex flex-col divide-y divide-border rounded-md border border-border">
        {routines.map((r) => (
          <li
            key={r.name}
            className="flex flex-wrap items-center gap-2 px-2 py-1.5 text-ui"
          >
            <span className="font-mono">{r.name}</span>
            <Badge tone="neutral">{t(`routines.trigger.${r.trigger}`)}</Badge>
            <span className="text-meta text-fg-muted">
              {r.cron
                ? `${r.cron} (${r.timezone ?? ""})`
                : r.channel
                  ? `#${r.channel}`
                  : r.action
                    ? r.action
                    : ""}
              {" → "}
              {r.agent ? `@${r.agent}` : r.tool}
              {r.output ? ` · #${r.output}` : ""}
            </span>
            {r.next_run_at ? (
              <span className="text-meta text-fg-muted">
                {t("routines.next", {
                  when: fullTime(r.next_run_at, i18n.language),
                })}
              </span>
            ) : null}
            {r.trigger === "webhook" ? (
              <Button
                size="sm"
                variant="ghost"
                loading={webhook.isPending}
                onClick={() =>
                  webhook.mutate(r.name, {
                    onSuccess: (w) =>
                      setAddress({
                        routine: r.name,
                        url: window.location.origin + w.path,
                      }),
                    onError: (e) => toast(e.message, { tone: "danger" }),
                  })
                }
              >
                {r.has_webhook
                  ? t("routines.newWebhook")
                  : t("routines.webhook")}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
      {address ? (
        <Notice tone="warning">
          <span className="block">
            {t("routines.webhookOnce", { routine: address.routine })}
          </span>
          <code className="block font-mono text-meta break-all select-all">
            {address.url}
          </code>
        </Notice>
      ) : null}
      {runs.length ? (
        <ul className="mt-2 flex flex-col gap-1 text-meta">
          {runs.slice(0, 10).map((run) => (
            <li key={run.id} className="flex flex-wrap items-center gap-2">
              <span className="text-fg-muted">
                {fullTime(run.created_at, i18n.language)}
              </span>
              <span className="font-mono">{run.routine}</span>
              <Badge tone={runTone[run.state]}>
                {t(`routines.state.${run.state}`)}
              </Badge>
              {run.error ? (
                <span className="truncate text-danger">{run.error}</span>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
    </details>
  );
}
