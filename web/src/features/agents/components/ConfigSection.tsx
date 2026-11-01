import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Lock } from "lucide-react";
import { Badge } from "@/shared/ui";
import type { AgentOverview } from "../model";
import { Section } from "./Section";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[minmax(7rem,auto)_1fr] gap-2 py-1">
      <dt className="text-fg-subtle">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

/**
 * 6. Konfiguráció: read-only, fixed at creation (docs/01 "Agent életciklus").
 * The assignment comes only to admins; others see a note.
 */
export function ConfigSection({ overview, onDetails }: { overview: AgentOverview; onDetails: () => void }) {
  const { t } = useTranslation("agents");
  const a = overview.assignment;
  const onOff = (v: boolean | undefined) => (v ? t("overview.config.on") : t("overview.config.off"));
  return (
    <Section
      id="config"
      title={t("overview.config.title")}
      aside={
        <button type="button" onClick={onDetails} className="shrink-0 text-meta text-accent-text hover:underline">
          {t("overview.config.details")}
        </button>
      }
    >
      <p className="mb-2 flex items-center gap-1.5 text-meta text-fg-muted">
        <Lock aria-hidden className="size-3.5" />
        {t("overview.config.locked")}
      </p>
      {a ? (
        <dl className="divide-y divide-border">
          <Row label={t("overview.config.root")}>
            <code className="font-mono text-meta">{a.root_dir || "–"}</code>
          </Row>
          {a.worktree_repo ? (
            <Row label={t("overview.config.worktree")}>
              <code className="font-mono text-meta">{a.worktree_repo}</code>
              {a.worktree_branch ? <span className="ml-1 font-mono text-meta text-fg-muted">({a.worktree_branch})</span> : null}
            </Row>
          ) : null}
          <Row label={t("overview.config.grants")}>
            {a.grants.length ? (
              <ul className="flex flex-col gap-1">
                {a.grants.map((g) => (
                  <li key={g.id} className="flex items-center gap-1.5">
                    <Badge tone={g.mode === "write" ? "warning" : "neutral"}>{t(`overview.config.${g.mode}`)}</Badge>
                    <code className="min-w-0 truncate font-mono text-meta">{g.path}</code>
                  </li>
                ))}
              </ul>
            ) : (
              <span className="text-fg-muted">{t("overview.config.noGrants")}</span>
            )}
          </Row>
          <Row label={t("overview.config.mode")}>{a.sandbox ? t("overview.sandbox") : t("overview.fullAccess")}</Row>
          <Row label={t("overview.config.webRead")}>{onOff(a.allow_web_read)}</Row>
          <Row label={t("overview.config.approvals")}>{a.approvals}</Row>
          <Row label={t("overview.config.permissionMode")}>{a.permission_mode}</Row>
          <Row label={t("overview.config.client")}>{a.client}</Row>
          <Row label={t("overview.config.model")}>{a.model}</Row>
          {a.work_model ? <Row label={t("overview.config.workModel")}>{a.work_model}</Row> : null}
        </dl>
      ) : (
        <p className="text-fg-muted">{t("overview.config.hidden")}</p>
      )}
    </Section>
  );
}
