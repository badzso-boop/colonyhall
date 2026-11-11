import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { hueVar } from "@/shared/lib";
import { Tooltip } from "@/shared/ui";
import { OTHER_KEY, shortTokens, type StackDay } from "../model";

export type Series = { key: string; label: string; color: string };

/** A chart colour for a series: its hue, or the recessive one for "Egyéb". */
export function seriesColor(key: string, hue: string | undefined): string {
  if (key === OTHER_KEY) return "var(--color-fg-subtle)";
  return hueVar(hue) ?? "var(--color-fg-muted)";
}

const shortDay = (day: string, locale: string) => new Date(`${day}T12:00:00Z`).toLocaleDateString(locale, { month: "short", day: "numeric" });

export function Legend({ series }: { series: Series[] }) {
  if (series.length < 2) return null;
  return (
    <ul className="flex flex-wrap gap-x-3 gap-y-1 text-meta text-fg-muted">
      {series.map((s) => (
        <li key={s.key} className="flex min-w-0 items-center gap-1.5">
          <span aria-hidden className="size-2.5 shrink-0 rounded-sm" style={{ background: s.color }} />
          <span className="truncate">{s.label}</span>
        </li>
      ))}
    </ul>
  );
}

/**
 * Stacked bars per day on one axis, 2 px surface gaps between the segments and
 * a tooltip per column. The agent table below the chart is its table view.
 */
export function StackedBars({ days, series, label, format }: { days: StackDay[]; series: Series[]; label: string; format: (n: number) => string }) {
  const { i18n } = useTranslation();
  const locale = localeOf(i18n.language);
  const max = Math.max(1, ...days.map((d) => d.total));
  const byKey = new Map(series.map((s) => [s.key, s]));
  const middle = days[Math.floor((days.length - 1) / 2)];
  const first = days[0];
  const last = days[days.length - 1];
  return (
    <figure className="flex min-w-0 flex-col gap-2">
      <Legend series={series} />
      <div className="flex min-w-0 gap-2">
        <div aria-hidden className="tabular flex h-44 w-10 shrink-0 flex-col justify-between text-right text-meta text-fg-subtle">
          <span>{format(max)}</span>
          <span>{format(max / 2)}</span>
          <span>0</span>
        </div>
        <div role="img" aria-label={label} className="relative flex h-44 min-w-0 flex-1 items-end gap-px border-b border-border">
          <span aria-hidden className="absolute inset-x-0 top-0 border-t border-border/60" />
          <span aria-hidden className="absolute inset-x-0 top-1/2 border-t border-border/60" />
          {days.map((d) => (
            <Tooltip
              key={d.day}
              content={
                <span className="flex flex-col gap-0.5">
                  <b>
                    {shortDay(d.day, locale)}: {format(d.total)}
                  </b>
                  {d.segments.map((s) => (
                    <span key={s.key} className="flex items-center gap-1.5">
                      <span aria-hidden className="size-2 rounded-sm" style={{ background: byKey.get(s.key)?.color }} />
                      {byKey.get(s.key)?.label}: {format(s.value)}
                    </span>
                  ))}
                </span>
              }
            >
              <span className="relative z-10 flex h-full min-w-0 flex-1 flex-col justify-end gap-0.5 hover:bg-surface-sunken" tabIndex={days.length <= 31 ? 0 : undefined}>
                {[...d.segments].reverse().map((s, i) => (
                  <span
                    key={s.key}
                    className={i === 0 ? "w-full rounded-t-sm" : "w-full"}
                    style={{ height: `${(s.value / max) * 100}%`, minHeight: 2, background: byKey.get(s.key)?.color }}
                  />
                ))}
              </span>
            </Tooltip>
          ))}
        </div>
      </div>
      <div aria-hidden className="tabular flex justify-between pl-12 text-meta text-fg-subtle">
        <span>{first ? shortDay(first.day, locale) : ""}</span>
        {days.length > 4 && middle ? <span>{shortDay(middle.day, locale)}</span> : null}
        <span>{last && last !== first ? shortDay(last.day, locale) : ""}</span>
      </div>
      <ul className="sr-only">
        {days.map((d) => (
          <li key={d.day}>
            {d.day}: {d.segments.map((s) => `${byKey.get(s.key)?.label} ${format(s.value)}`).join(", ") || "0"}
          </li>
        ))}
      </ul>
    </figure>
  );
}

/**
 * One series per day, no legend, the row names it: an agent's tokens, or the
 * database size. fromMin scales between the smallest and largest value, so a
 * slowly growing total still shows its shape.
 */
export function Sparkline({ values, label, format, fromMin = false }: { values: number[]; label: string; format?: (n: number) => string; fromMin?: boolean }) {
  const { i18n } = useTranslation();
  const locale = localeOf(i18n.language);
  const show = format ?? ((n: number) => shortTokens(n, locale));
  const base = fromMin ? Math.min(...values) : 0;
  const span = Math.max(1, Math.max(...values) - base);
  return (
    <span role="img" aria-label={`${label}: ${values.map(show).join(", ")}`} className="flex h-6 w-24 items-end gap-px">
      {values.map((v, i) => (
        <span key={i} className="min-w-0 flex-1 rounded-t-[1px] bg-info" style={{ height: `${v > 0 ? Math.max(8, ((v - base) / span) * 100) : 0}%` }} />
      ))}
    </span>
  );
}

/** A usage ring: amber from 80 %, always with its number and name next to it. */
export function LimitRing({ pct, label, title }: { pct: number; label: string; title: string }) {
  const warn = pct >= 80;
  return (
    <Tooltip content={title}>
      <span role="img" aria-label={title} tabIndex={0} className="inline-flex items-center gap-1.5 text-meta text-fg-muted">
        <span
          className="grid size-7 place-items-center rounded-full"
          style={{ background: `conic-gradient(var(--color-${warn ? "warning" : "info"}) ${pct}%, var(--color-border) 0)` }}
        >
          <span className="size-5 rounded-full bg-surface" />
        </span>
        <span className="flex flex-col leading-tight">
          <span className="tabular font-semibold text-fg">{pct}%</span>
          <span>{label}</span>
        </span>
      </span>
    </Tooltip>
  );
}
