// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { KeyRound } from "lucide-react";
import { ApiError } from "@/shared/api";
import { fullTime } from "@/shared/lib";
import { Button, Dialog, Input, Notice, PageHeader } from "@/shared/ui";
import { useOperatorSession, useRenewBackupCodes } from "../api";
import { normaliseCode } from "../model";

/** Fiókom: the operator, the session's end and the backup codes. */
export default function AccountPage() {
  const { t, i18n } = useTranslation("operator");
  const session = useOperatorSession();
  const [renewing, setRenewing] = useState(false);
  const s = session.data;
  if (!s) return null;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader icon={<KeyRound className="size-5" />} title={t("account.title")} />
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-3 sm:p-4">
        <dl className="grid max-w-2xl grid-cols-1 gap-x-6 gap-y-2 rounded-lg border border-border bg-surface p-3 text-ui sm:grid-cols-[12rem_1fr]">
          <dt className="text-fg-muted">{t("account.handle")}</dt>
          <dd>@{s.operator.handle}</dd>
          <dt className="text-fg-muted">{t("account.name")}</dt>
          <dd>{s.operator.display_name}</dd>
          <dt className="text-fg-muted">{t("account.sessionEnds")}</dt>
          <dd>{t("account.sessionEndsValue", { when: fullTime(s.expires_at, i18n.language) })}</dd>
          <dt className="text-fg-muted">{t("account.backupCodes")}</dt>
          <dd className="tabular">{t("account.backupCodesLeft", { count: s.backup_codes_left })}</dd>
        </dl>
        {s.backup_codes_left <= 3 ? <Notice tone="warning">{t("account.backupCodesLow")}</Notice> : null}
        <div>
          <Button onClick={() => setRenewing(true)}>{t("account.renew")}</Button>
        </div>
        <p className="max-w-2xl text-meta text-fg-subtle">{t("account.totpHelp")}</p>
      </div>
      {renewing ? <RenewDialog onClose={() => setRenewing(false)} /> : null}
    </div>
  );
}

function RenewDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation("operator");
  const renew = useRenewBackupCodes();
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    renew.mutate({ password, code: normaliseCode(code) }, { onError: () => setCode("") });
  };
  const err = renew.error;
  const error = err ? (err instanceof ApiError && err.status === 401 ? t("login.failed") : err.message) : null;
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("account.renew")} closeLabel={t("common:close")}
      footer={
        renew.data ? (
          <Button variant="primary" onClick={onClose}>{t("done")}</Button>
        ) : (
          <>
            <Button onClick={onClose}>{t("cancel")}</Button>
            <Button type="submit" form="operator-renew" variant="primary" loading={renew.isPending}>
              {t("account.renewSubmit")}
            </Button>
          </>
        )
      }>
      {renew.data ? (
        <div className="flex flex-col gap-3 text-ui">
          <Notice tone="warning">{t("account.codesOnce")}</Notice>
          <ul className="grid grid-cols-2 gap-1 rounded-md border border-border bg-surface-sunken p-3 font-mono select-all">
            {renew.data.codes.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        </div>
      ) : (
        <form id="operator-renew" onSubmit={submit} className="flex flex-col gap-3 text-ui">
          <p>{t("account.renewBody")}</p>
          <label className="flex flex-col gap-1">
            {t("login.password")}
            <Input type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
          <label className="flex flex-col gap-1">
            {t("account.totp")}
            <Input autoComplete="one-time-code" inputMode="numeric" required value={code} onChange={(e) => setCode(e.target.value)} />
          </label>
          {error ? <p role="alert" className="text-danger">{error}</p> : null}
        </form>
      )}
    </Dialog>
  );
}
