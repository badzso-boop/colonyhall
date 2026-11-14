// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Gavel, Search } from "lucide-react";
import { useActors } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { fullTime, relativeTime } from "@/shared/lib";
import { Badge, Button, EmptyState, Input, Markdown, PageHeader, Skeleton } from "@/shared/ui";
import { useDecisions } from "../api";

const Page = 50;

/** Döntések: the decision log, newest first, read-only (round 32, navigation). */
export default function DecisionsPage() {
  const { t, i18n } = useTranslation("decisions");
  const locale = localeOf(i18n.language);
  const actors = useActors();
  const [q, setQ] = useState("");
  const [limit, setLimit] = useState(Page);
  const list = useDecisions(q.trim(), limit);
  const now = new Date();
  return (
    <div className="flex h-full flex-col">
      <PageHeader icon={<Gavel className="size-5" />} title={t("title")} />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-3xl flex-col gap-3 p-4">
          <label className="relative">
            <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
            <Input type="search" aria-label={t("search")} placeholder={t("search")} value={q} onChange={(e) => setQ(e.target.value)} className="pl-8" />
          </label>
          <p className="text-meta text-fg-subtle">{t("hint")}</p>
          {list.isPending ? <Skeleton className="h-24 w-full" /> : null}
          {list.data && list.data.length === 0 ? <EmptyState icon={<Gavel className="size-8" />} title={q ? t("noMatch") : t("empty")} /> : null}
          {list.data?.map((d) => {
            const author = actors.data?.get(d.author_id);
            const href = `/c/${d.channel_id}${d.thread_root_id ? `?thread=${d.thread_root_id}` : ""}`;
            return (
              <article key={d.id} id={d.formatted_number} aria-label={d.formatted_number} className="rounded-lg border border-border bg-surface p-3">
                <header className="mb-1.5 flex flex-wrap items-center gap-2 text-meta text-fg-muted">
                  <Badge tone="accent">{d.formatted_number}</Badge>
                  {author ? <span>@{author.handle}</span> : null}
                  <time dateTime={d.created_at} title={fullTime(d.created_at, locale)}>
                    {relativeTime(d.created_at, now, locale)}
                  </time>
                  {d.supersedes ? <Badge tone="neutral">{t("supersedes", { n: d.supersedes })}</Badge> : null}
                  {d.superseded_by ? <Badge tone="warning">{t("supersededBy", { n: d.superseded_by })}</Badge> : null}
                  <Link to={href} className="ml-auto hover:text-fg hover:underline">
                    {t("open")}
                  </Link>
                </header>
                <Markdown text={d.text} className={d.superseded_by ? "text-fg-muted" : undefined} copyLabel={t("copy")} copiedLabel={t("copied")} />
                {d.rationale ? (
                  <div className="mt-2 border-l-2 border-border pl-3 text-ui text-fg-muted">
                    <p className="text-meta font-semibold">{t("rationale")}</p>
                    <Markdown text={d.rationale} copyLabel={t("copy")} copiedLabel={t("copied")} />
                  </div>
                ) : null}
              </article>
            );
          })}
          {list.data && list.data.length >= limit ? (
            <Button className="self-center" loading={list.isFetching} onClick={() => setLimit(limit + Page)}>
              {t("more")}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
