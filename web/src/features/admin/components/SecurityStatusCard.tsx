// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { fullTime } from "@/shared/lib";
import { securityTone, type SecurityStatus } from "../model";
import { StatusCard, StatusRow } from "./StatusCard";

/** The "Biztonság" card of the status page (round 40): lockouts of the last 24 hours, the allowlist and the trusted proxies. */
export function SecurityStatusCard({ security }: { security: SecurityStatus }) {
  const { t, i18n } = useTranslation("admin");
  const when = (at: string) => fullTime(at, i18n.language);
  return (
    <StatusCard title={t("status.security.title")} tone={securityTone(security)}>
      <StatusRow label={t("status.security.lockouts")} value={security.lockouts_24h} />
      {security.lockouts.length ? (
        <ul className="flex flex-col gap-1 border-t border-border pt-1">
          {security.lockouts.map((l) => (
            <li key={`${l.scope}-${l.handle}-${l.ip}-${l.at}`} className="flex flex-col">
              <span className="font-medium text-fg">
                {l.handle ? `@${l.handle}` : "–"} · {l.ip || t("status.security.noIp")} · {t(`status.security.scope.${l.scope}`)}
              </span>
              <span className="text-meta">{t("status.security.until", { at: when(l.at), until: when(l.until) })}</span>
            </li>
          ))}
        </ul>
      ) : null}
      <StatusRow
        label={t("status.security.allowlist")}
        value={security.allowlist.length ? security.allowlist.join(", ") : t("status.security.none")}
      />
      <StatusRow label={t("status.security.trustedProxies")} value={t(security.trusted_proxies ? "settings.yes" : "settings.no")} />
    </StatusCard>
  );
}
