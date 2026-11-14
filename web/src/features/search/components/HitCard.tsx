// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { localeOf } from "@/shared/i18n";
import { fullTime, relativeTime } from "@/shared/lib";
import { formatScore, hitHref, scoreParts, type SearchHit } from "../model";
import { Snippet } from "./Snippet";

/** The stage scores of a hit, compactly ("kulcsszó 0.12 · jelentés 0.81 · RRF 0.032"); nothing without scores. */
export function ScoreLine({ hit }: { hit: SearchHit }) {
  const { t } = useTranslation("search");
  const parts = scoreParts(hit.scores);
  if (!parts.length) return null;
  return (
    <p aria-label={t("scores.label")} className="tabular text-meta text-fg-subtle">
      {parts.map((p) => `${t(`scores.${p.stage}`)} ${formatScore(p.value)}`).join(" · ")}
    </p>
  );
}

/** One search hit: its context line, date, snippet and a link to where it lives. */
export function HitCard({ hit, now }: { hit: SearchHit; now: Date }) {
  const { t, i18n } = useTranslation("search");
  const locale = localeOf(i18n.language);
  const href = hitHref(hit);
  return (
    <article aria-label={hit.header} className="flex min-w-0 flex-col gap-1 rounded-lg border border-border bg-surface p-3">
      <header className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-meta text-fg-muted">
        <span className="min-w-0 flex-1 break-words">{hit.header}</span>
        <time dateTime={hit.at} title={fullTime(hit.at, locale)}>
          {relativeTime(hit.at, now, locale)}
        </time>
        {href ? (
          <Link to={href} aria-label={`${t("open")}: ${hit.header}`} className="font-medium text-fg hover:underline">
            {t("open")}
          </Link>
        ) : null}
      </header>
      <Snippet text={hit.snippet} className="text-ui break-words whitespace-pre-wrap text-fg" />
      <ScoreLine hit={hit} />
    </article>
  );
}
