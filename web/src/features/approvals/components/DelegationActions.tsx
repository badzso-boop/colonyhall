import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { shortTime } from "@/shared/lib";
import { Badge, Button, toast } from "@/shared/ui";
import { useAnswerQuestion } from "@/features/questions";
import { useDelegations } from "../api";
import { DELEGATION_APPROVE, DELEGATION_DENY, delegationOf } from "../model";

/**
 * Under the source agent's message (rounds 24–26): while the delegation
 * waits, admins get Jóváhagyom / Elutasítom (the answer to its question),
 * others "jóváhagyásra vár"; afterwards the state chip.
 */
export function DelegationActions({ message, admin }: { message: { id: string; client_msg_id?: string }; admin: boolean }) {
  const { t, i18n } = useTranslation("approvals");
  const locale = localeOf(i18n.language);
  const delegations = useDelegations(true);
  const answer = useAnswerQuestion();
  const d = delegations.data ? delegationOf(delegations.data, message) : undefined;
  if (!d) return null;

  const decide = (value: string) =>
    answer.mutate({ id: d.question_id, answer: value }, { onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }) });
  const who = d.decided_by_handle ? `@${d.decided_by_handle}` : "";
  const when = d.decided_at ? shortTime(d.decided_at, locale) : "";
  const target = d.to_agent_handle ? `@${d.to_agent_handle}` : "";

  return (
    <div role="group" aria-label={t("delegation.label", { target })} className="mt-1.5 flex flex-wrap items-center gap-2 text-meta">
      {d.state === "pending" ? (
        admin ? (
          <>
            <span className="text-fg-muted">{t("delegation.ask", { target })}</span>
            <Button size="sm" variant="primary" disabled={answer.isPending} onClick={() => decide(DELEGATION_APPROVE)}>
              {t("delegation.approve")}
            </Button>
            <Button size="sm" variant="danger" disabled={answer.isPending} onClick={() => decide(DELEGATION_DENY)}>
              {t("delegation.deny")}
            </Button>
          </>
        ) : (
          <Badge tone="warning">{t("delegation.pending")}</Badge>
        )
      ) : d.state === "approved" ? (
        <Badge tone="success">
          {d.approved_by_grant ? t("delegation.approvedByGrant") : t("delegation.approved", { who, when })}
        </Badge>
      ) : d.state === "denied" ? (
        <Badge tone="danger">{t("delegation.denied", { who, when })}</Badge>
      ) : (
        <Badge>{t(`delegation.${d.state}`)}</Badge>
      )}
    </div>
  );
}
