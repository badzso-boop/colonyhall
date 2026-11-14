// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Handshake } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { shortTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { Button, toast } from "@/shared/ui";
import { useActiveGrants, useRevokeGrant } from "../api";
import { grantsHere, type Grant } from "../model";

function Row({ grant, admin }: { grant: Grant; admin: boolean }) {
  const { t, i18n } = useTranslation("approvals");
  const revoke = useRevokeGrant();
  const [confirm, setConfirm] = useState(false);
  return (
    <li className="flex flex-wrap items-center gap-2">
      <Handshake aria-hidden className="size-4 shrink-0 text-info" />
      <span className="min-w-0 flex-1">
        {t("grant.text", { a: grant.agent_a_handle, b: grant.agent_b_handle, until: shortTime(grant.effective_expires_at, localeOf(i18n.language)) })}
      </span>
      {admin && !confirm ? (
        <Button size="sm" variant="ghost" onClick={() => setConfirm(true)}>
          {t("grant.revoke")}
        </Button>
      ) : null}
      {admin && confirm ? (
        <span role="group" aria-label={t("grant.confirm")} className="flex items-center gap-1.5">
          <span>{t("grant.confirm")}</span>
          <Button
            size="sm"
            variant="danger"
            loading={revoke.isPending}
            onClick={() =>
              revoke.mutate(grant.id, {
                onSuccess: () => setConfirm(false),
                onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }),
              })
            }
          >
            {t("grant.yes")}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setConfirm(false)}>
            {t("grant.cancel")}
          </Button>
        </span>
      ) : null}
    </li>
  );
}

/**
 * Under the conversation header (round 25): "🤝 @a ↔ @b: engedély, lejár
 * 17:25" for each active grant of this DM or thread; admins can revoke it
 * after an in-page confirmation. Humans only (the list is theirs).
 */
export function GrantBar({ channelId, threadRootId = null, isDm, admin }: { channelId: string; threadRootId?: string | null; isDm: boolean; admin: boolean }) {
  const now = useNow(30_000);
  const grants = useActiveGrants(true);
  const here = grantsHere(grants.data ?? [], channelId, threadRootId, isDm, now);
  if (!here.length) return null;
  return (
    <ul role="status" className="flex shrink-0 flex-col gap-1 border-b border-border bg-info-soft px-4 py-1.5 text-ui text-fg">
      {here.map((g) => (
        <Row key={g.id} grant={g} admin={admin} />
      ))}
    </ul>
  );
}
