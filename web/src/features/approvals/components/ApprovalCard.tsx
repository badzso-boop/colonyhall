// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Clock, ShieldQuestion } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime, messageTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { Badge, Button, Sheet, Skeleton, toast } from "@/shared/ui";
import { useActors } from "@/features/identity";
import { useApproval, useDecideApproval } from "../api";
import { inputSummary, prettyInput, timeLeft, type Approval } from "../model";

const stateTone = { allowed: "success", denied: "danger", expired: "neutral", pending: "warning" } as const;

function Details({ approval, open, onOpenChange }: { approval: Approval; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { t } = useTranslation("approvals");
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={t("approval.detailsTitle", { tool: approval.tool_name })} closeLabel={t("common:close")}>
      <p className="mb-2 text-ui text-fg-muted">{t("approval.masked")}</p>
      {/* The input as the server sent it (masked there); never unmasked or interpreted here. */}
      <pre className="overflow-auto rounded-md bg-surface-sunken p-2.5 font-mono text-meta break-all whitespace-pre-wrap">{prettyInput(approval.input)}</pre>
    </Sheet>
  );
}

/**
 * A permission request in a message ([approval:<id>], round 05), for
 * humans: the tool, a one-line summary and "Részletek"; while pending,
 * Engedélyezem / Elutasítom and the auto-deny countdown; afterwards who
 * decided and when. approval.* events refetch it (events.ts).
 */
export function ApprovalCard({ id }: { id: string }) {
  const { t, i18n } = useTranslation("approvals");
  const locale = localeOf(i18n.language);
  const now = useNow(1000);
  const approval = useApproval(id, true);
  const decide = useDecideApproval(id);
  const actors = useActors();
  const [details, setDetails] = useState(false);

  if (approval.isPending) return <Skeleton className="mt-1.5 h-14 max-w-md" />;
  if (!approval.data) return null;
  const a = approval.data;
  // A pending request past its deadline is denied by the server; show it as expired until the event arrives.
  const state = a.state === "pending" && new Date(a.expires_at).getTime() <= now.getTime() ? "expired" : a.state;
  const decider = a.decided_by ? actors.data?.get(a.decided_by) : undefined;

  const run = (d: "allow" | "deny") =>
    decide.mutate(d, { onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }) });

  return (
    <section aria-label={t("approval.label", { tool: a.tool_name })} className={cn("mt-1.5 flex max-w-xl flex-col gap-1.5 rounded-lg border bg-surface-raised p-2.5", state === "pending" ? "border-warning" : "border-border")}>
      <div className="flex flex-wrap items-center gap-2 text-ui">
        <ShieldQuestion aria-hidden className="size-4 text-warning" />
        <span className="font-semibold">{t("approval.title")}</span>
        <code className="rounded-sm bg-surface-sunken px-1 font-mono text-meta">{a.tool_name}</code>
        <Badge tone={stateTone[state as keyof typeof stateTone] ?? "neutral"}>{t(`approval.state.${state}`, { defaultValue: state })}</Badge>
        {state === "pending" ? (
          <span className="tabular ml-auto inline-flex items-center gap-1 text-meta text-fg-muted" title={fullTime(a.expires_at, locale)}>
            <Clock aria-hidden className="size-3.5" />
            {t("approval.autoDeny", { left: timeLeft(a.expires_at, now) })}
          </span>
        ) : null}
      </div>
      <p className="truncate font-mono text-meta text-fg-muted" title={inputSummary(a.input, 400)}>
        {inputSummary(a.input)}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="ghost" onClick={() => setDetails(true)}>
          {t("approval.details")}
        </Button>
        {state === "pending" ? (
          <>
            <Button size="sm" variant="primary" loading={decide.isPending && decide.variables === "allow"} disabled={decide.isPending} onClick={() => run("allow")}>
              {t("approval.allow")}
            </Button>
            <Button size="sm" variant="danger" loading={decide.isPending && decide.variables === "deny"} disabled={decide.isPending} onClick={() => run("deny")}>
              {t("approval.deny")}
            </Button>
          </>
        ) : a.decided_at ? (
          <span className="text-meta text-fg-muted">
            {t(state === "allowed" ? "approval.allowedBy" : "approval.deniedBy", {
              who: decider ? `@${decider.handle}` : t("approval.someone"),
              when: messageTime(a.decided_at, now, locale),
            })}
          </span>
        ) : null}
      </div>
      <Details approval={a} open={details} onOpenChange={setDetails} />
    </section>
  );
}
