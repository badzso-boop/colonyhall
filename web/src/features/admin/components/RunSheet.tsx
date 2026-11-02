import { useTranslation } from "react-i18next";
import { ErrorChip, StepList } from "@/features/agents";
import type { Schemas } from "@/shared/api";
import { localeOf } from "@/shared/i18n";
import { fullTime } from "@/shared/lib";
import { Sheet } from "@/shared/ui";
import { formatDuration, formatHuf, formatUsd, toHuf } from "../model";

/** One run in a Sheet: the steps, the session, the context size and, for failures, the error details. */
export function RunSheet({ run, agent, channel, rate, onClose }: { run: Schemas["Run"]; agent: string; channel?: string; rate?: number; onClose: () => void }) {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const num = (n: number) => n.toLocaleString(locale);
  const huf = run.estimated_usd != null ? toHuf(run.estimated_usd, rate) : null;
  const rows: [string, string][] = [
    [t("runs.sheet.agent"), agent],
    [t("runs.sheet.phase"), t(`runs.phase.${run.phase}`)],
    [t("runs.sheet.channel"), channel ?? "–"],
    [t("runs.sheet.client"), `${run.client} · ${run.model || "–"}`],
    [t("runs.sheet.started"), fullTime(run.created_at, i18n.language)],
    [t("runs.sheet.duration"), formatDuration(run.duration_ms)],
    [t("runs.sheet.input"), num(run.input_tokens)],
    [t("runs.sheet.output"), num(run.output_tokens)],
    [t("runs.sheet.cache"), `${num(run.cache_read_tokens)} / ${num(run.cache_write_tokens)}`],
    [t("runs.sheet.context"), run.context_tokens ? num(run.context_tokens) : "–"],
    [t("runs.sheet.session"), run.session_id || "–"],
    [
      t("runs.sheet.cost"),
      run.estimated_usd == null
        ? t("runs.noPrice")
        : `~${formatUsd(run.estimated_usd, locale)}${huf != null ? ` · ~${formatHuf(huf, locale)}` : ""}${run.billing === "subscription" ? ` (${t("runs.subscriptionShort")})` : ""}`,
    ],
  ];
  return (
    <Sheet open onOpenChange={(o) => !o && onClose()} title={t("runs.sheet.title", { agent })} closeLabel={t("close")}>
      <div className="flex flex-col gap-4 overflow-y-auto p-4">
        {run.error_id ? (
          <p className="flex items-center gap-2 text-ui">
            {t("runs.sheet.error")} <ErrorChip errorId={run.error_id} human />
          </p>
        ) : null}
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-ui">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-fg-subtle">{k}</dt>
              <dd className="min-w-0 break-words">{v}</dd>
            </div>
          ))}
        </dl>
        {run.web_reads?.length ? (
          <section>
            <h3 className="mb-1 font-semibold">{t("runs.sheet.web", { count: run.web_reads.length })}</h3>
            <ul className="flex flex-col gap-0.5 text-meta break-all text-fg-muted">
              {run.web_reads.map((u) => (
                <li key={u}>{u}</li>
              ))}
            </ul>
          </section>
        ) : null}
        <section>
          <h3 className="mb-1 font-semibold">{t("runs.sheet.steps")}</h3>
          {run.steps?.length ? <StepList steps={run.steps} /> : <p className="text-ui text-fg-muted">{t("runs.sheet.noSteps")}</p>}
        </section>
      </div>
    </Sheet>
  );
}
