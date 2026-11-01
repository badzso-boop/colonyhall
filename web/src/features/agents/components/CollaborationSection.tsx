import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { useNow } from "@/shared/hooks";
import { localeOf } from "@/shared/i18n";
import { relativeTime } from "@/shared/lib";
import { Button, toast } from "@/shared/ui";
import { useDelegationGrants, useReleaseLease, useRevokeGrant } from "../api";
import { grantsOf, placeHref, type AgentOverview } from "../model";
import { Section } from "./Section";

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <h4 className="text-meta font-semibold text-fg-subtle">{title}</h4>
      {children}
    </div>
  );
}

/** The common channels (agents and humans alike). */
export function CommonChannels({ overview }: { overview: AgentOverview }) {
  const { t } = useTranslation("agents");
  const channels = overview.common_channels ?? [];
  return channels.length ? (
    <ul className="flex flex-wrap gap-1.5">
      {channels.map((c) => (
        <li key={c.id}>
          <Link to={`/c/${c.id}`} className="text-accent-text hover:underline">
            #{c.name || t("overview.now.channel")}
          </Link>
        </li>
      ))}
    </ul>
  ) : (
    <p className="text-fg-muted">{t("overview.collab.none")}</p>
  );
}

/**
 * 7. Együttműködés: delegation grants (admins revoke), active leases
 * (humans release), open questions and the common channels.
 */
export function CollaborationSection({ overview, admin }: { overview: AgentOverview; admin: boolean }) {
  const { t, i18n } = useTranslation("agents");
  const locale = localeOf(i18n.language);
  const now = useNow();
  const agentId = overview.actor.id;
  const grants = useDelegationGrants(admin);
  const revoke = useRevokeGrant();
  const release = useReleaseLease(agentId);
  const mine = grantsOf(grants.data ?? [], agentId);
  const leases = overview.collaboration?.active_leases ?? [];
  const questions = overview.collaboration?.open_questions ?? [];
  const when = (d: string) => relativeTime(d, now, locale);
  return (
    <Section id="collaboration" title={t("overview.collab.title")}>
      <div className="flex flex-col gap-3">
        {admin ? (
          <Group title={t("overview.collab.grants")}>
            {mine.length ? (
              <ul className="flex flex-col gap-1">
                {mine.map((g) => (
                  <li key={g.id} className="flex items-center gap-2">
                    <Link to={placeHref(g.channel_id, g.thread_root_id)} className="min-w-0 flex-1 truncate hover:underline">
                      {t("overview.collab.grantWith", { other: g.agent_a_id === agentId ? g.agent_b_handle : g.agent_a_handle, when: when(g.effective_expires_at) })}
                    </Link>
                    <Button size="sm" variant="danger" loading={revoke.isPending && revoke.variables === g.id} onClick={() => revoke.mutate(g.id, { onSuccess: () => toast(t("overview.collab.revoked")) })}>
                      {t("overview.collab.revoke")}
                    </Button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-fg-muted">{t("overview.collab.none")}</p>
            )}
          </Group>
        ) : null}
        <Group title={t("overview.collab.leases")}>
          {leases.length ? (
            <ul className="flex flex-col gap-1">
              {leases.map((l) => (
                <li key={l.id} className="flex items-center gap-2">
                  <code className="min-w-0 truncate font-mono text-meta">{l.resource}</code>
                  <span className="tabular text-meta text-fg-muted">{t("overview.collab.leaseExpires", { when: when(l.expires_at) })}</span>
                  <Button size="sm" className="ml-auto" loading={release.isPending && release.variables === l.id} onClick={() => release.mutate(l.id, { onSuccess: () => toast(t("overview.collab.released")) })}>
                    {t("overview.collab.release")}
                  </Button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-fg-muted">{t("overview.collab.none")}</p>
          )}
        </Group>
        <Group title={t("overview.collab.questions")}>
          {questions.length ? (
            <ul className="flex flex-col gap-1">
              {questions.map((q) => (
                <li key={q.id} className="flex flex-wrap items-baseline gap-x-2">
                  {/* The questions Sheet comes in round 34; until then the thread. */}
                  <Link to={placeHref(q.channel_id, q.thread_root_id)} className="font-mono font-semibold text-accent-text hover:underline">
                    {q.formatted_number}
                  </Link>
                  <span className="text-meta text-fg-muted">{t("overview.collab.questionTo", { to: q.to_actor_handle ? `@${q.to_actor_handle}` : t("overview.collab.toHumans") })}</span>
                  {q.deadline ? <span className="tabular text-meta text-fg-muted">{t("overview.collab.deadline", { when: when(q.deadline) })}</span> : null}
                  <span className="w-full truncate text-fg-muted">{q.text}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-fg-muted">{t("overview.collab.none")}</p>
          )}
        </Group>
        <Group title={t("overview.collab.channels")}>
          <CommonChannels overview={overview} />
        </Group>
      </div>
    </Section>
  );
}
