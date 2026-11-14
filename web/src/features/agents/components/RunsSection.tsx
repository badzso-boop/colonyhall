// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { CircleCheck, CircleX, Globe, OctagonPause } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { messageTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { Button, Dialog, Skeleton } from "@/shared/ui";
import { useAgentRuns, useRunError } from "../api";
import { formatDuration, placeHref, shortTokens, type AgentOverview } from "../model";
import { Section } from "./Section";

/** One row, from either a Run (humans: place and duration) or an OverviewRun. */
type Row = { id: string; time: string; status: string; tokens: number; webReads: number; errorId?: string | undefined; durationMs?: number | undefined; channelId?: string | undefined; threadRootId?: string | undefined };

const icons = {
  ok: <CircleCheck aria-hidden className="size-4 text-success" />,
  error: <CircleX aria-hidden className="size-4 text-danger" />,
  halted: <OctagonPause aria-hidden className="size-4 text-warning" />,
} as const;

function ErrorDialog({ errorId, onClose }: { errorId: string | null; onClose: () => void }) {
  const { t } = useTranslation("agents");
  const err = useRunError(errorId);
  return (
    <Dialog open={errorId !== null} onOpenChange={(o) => !o && onClose()} title={t("overview.runs.errorTitle")} closeLabel={t("overview.runs.close")} size="lg">
      {err.data ? (
        <div className="flex flex-col gap-2">
          <p className="text-body">{err.data.summary}</p>
          <pre className="max-h-80 overflow-auto rounded-md bg-surface-sunken p-2 font-mono text-meta break-all whitespace-pre-wrap">{err.data.detail}</pre>
        </div>
      ) : (
        <Skeleton className="h-4 w-2/3" />
      )}
    </Dialog>
  );
}

/** 4. Utolsó futások: the five latest; "Összes futás" goes to the dev page until round 35. */
export function RunsSection({ overview, human, channelName }: { overview: AgentOverview; human: boolean; channelName: (id: string) => string }) {
  const { t, i18n } = useTranslation("agents");
  const locale = localeOf(i18n.language);
  const now = useNow();
  const runs = useAgentRuns(overview.actor.id, human);
  const [errorId, setErrorId] = useState<string | null>(null);
  const rows: Row[] = runs.data
    ? runs.data.map((r) => ({
        id: r.id,
        time: r.created_at,
        status: r.status,
        tokens: r.input_tokens + r.output_tokens + r.cache_read_tokens + r.cache_write_tokens,
        webReads: r.web_reads?.length ?? 0,
        errorId: r.error_id,
        durationMs: r.duration_ms,
        channelId: r.channel_id,
        threadRootId: r.thread_root_id,
      }))
    : (overview.usage?.last_runs ?? []).map((r) => ({ id: r.id, time: r.time, status: r.result, tokens: r.tokens, webReads: r.web_reads?.length ?? 0, errorId: r.error_id ?? undefined }));
  const shown = rows.slice(0, 5);
  return (
    <Section
      id="runs"
      title={t("overview.runs.title")}
      aside={
        // The runs page comes in round 35; until then the dev page lists them.
        <a href="/" className="shrink-0 text-meta text-accent-text hover:underline">
          {t("overview.runs.all")}
        </a>
      }
    >
      {!shown.length ? <p className="text-fg-muted">{t("overview.runs.empty")}</p> : null}
      <ul className="flex flex-col divide-y divide-border">
        {shown.map((r) => (
          <li key={r.id} className="flex flex-wrap items-center gap-x-2 gap-y-0.5 py-1.5">
            <span title={t(`overview.runs.${r.status}`, { defaultValue: r.status })} className="flex items-center">
              {icons[r.status as keyof typeof icons] ?? icons.ok}
              <span className="sr-only">{t(`overview.runs.${r.status}`, { defaultValue: r.status })}</span>
            </span>
            <span className="tabular text-meta text-fg-muted">{messageTime(new Date(r.time), now, locale)}</span>
            {r.durationMs !== undefined ? <span className="tabular text-meta text-fg-muted">{formatDuration(r.durationMs)}</span> : null}
            <span className="tabular text-meta text-fg-muted">{t("overview.runs.tokens", { tokens: shortTokens(r.tokens) })}</span>
            {r.webReads > 0 ? (
              <span className="flex items-center gap-0.5 text-meta text-fg-muted" title={t("overview.runs.webReads", { count: r.webReads })}>
                <Globe aria-hidden className="size-3.5" />
                {r.webReads}
              </span>
            ) : null}
            {r.channelId ? (
              <Link to={placeHref(r.channelId, r.threadRootId)} className="min-w-0 truncate text-meta text-accent-text hover:underline">
                #{channelName(r.channelId)}
              </Link>
            ) : null}
            {human && r.status === "error" && r.errorId ? (
              <Button size="sm" variant="ghost" className="ml-auto" onClick={() => setErrorId(r.errorId!)}>
                {t("overview.runs.errorDetails")}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
      <ErrorDialog errorId={errorId} onClose={() => setErrorId(null)} />
    </Section>
  );
}
