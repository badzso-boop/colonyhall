import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Gavel } from "lucide-react";
import { Badge, Markdown, Popover, PopoverContent, PopoverTrigger, Skeleton } from "@/shared/ui";
import { useDecision } from "../api";

/**
 * A D-12 chip in a message: a popover with the decision's text and
 * rationale (humans; the list endpoint is theirs). Loaded when opened.
 */
export function DecisionChip({ number }: { number: string }) {
  const { t } = useTranslation("decisions");
  const [open, setOpen] = useState(false);
  const decision = useDecision(number, open);
  const d = decision.data;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="mx-0.5 inline-flex items-center gap-1 rounded-full border border-border bg-surface px-1.5 align-baseline font-mono text-meta font-semibold text-accent-text hover:border-border-strong"
        >
          <Gavel aria-hidden className="size-3" />
          {number}
        </button>
      </PopoverTrigger>
      <PopoverContent label={number} className="w-96">
        {decision.isPending ? (
          <Skeleton className="h-12" />
        ) : !d ? (
          <p className="text-fg-muted">{t("chip.notFound", { n: number })}</p>
        ) : (
          <div className="flex flex-col gap-2">
            <p className="flex flex-wrap items-center gap-2 text-meta">
              <Badge tone="accent">{d.formatted_number}</Badge>
              {d.superseded_by ? <Badge tone="warning">{t("supersededBy", { n: d.superseded_by })}</Badge> : null}
              <Link to={`/c/${d.channel_id}${d.thread_root_id ? `?thread=${d.thread_root_id}` : ""}`} onClick={() => setOpen(false)} className="ml-auto text-accent-text hover:underline">
                {t("open")}
              </Link>
            </p>
            <Markdown text={d.text} copyLabel={t("copy")} copiedLabel={t("copied")} />
            {d.rationale ? (
              <div className="border-l-2 border-border pl-3 text-fg-muted">
                <p className="text-meta font-semibold">{t("rationale")}</p>
                <Markdown text={d.rationale} copyLabel={t("copy")} copiedLabel={t("copied")} />
              </div>
            ) : null}
          </div>
        )}
      </PopoverContent>
    </Popover>
  );
}
