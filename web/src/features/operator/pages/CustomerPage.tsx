// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";
import { ArrowLeft, Building2 } from "lucide-react";
import { SecretOnce } from "@/features/admin";
import { fullTime } from "@/shared/lib";
import { Button, Dialog, Notice, PageHeader, Skeleton, Textarea } from "@/shared/ui";
import { useCustomer, useInviteOwner, useRestoreCustomer, useSuspendCustomer } from "../api";
import { OwnerBadge, StateBadge, StorageText } from "../components/CustomerBadges";
import { canInvite, type Customer } from "../model";

type Action = "suspend" | "restore" | "invite" | null;

/** One customer: account data, its owner invite, suspend and restore. */
export default function CustomerPage() {
  const { t, i18n } = useTranslation("operator");
  const { id = "" } = useParams();
  const customer = useCustomer(id);
  const [action, setAction] = useState<Action>(null);
  const c = customer.data;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        icon={<Building2 className="size-5" />}
        title={c?.name ?? t("customers.title")}
        actions={
          <Link to="/" className="inline-flex items-center gap-1 text-ui text-fg-muted hover:text-fg">
            <ArrowLeft aria-hidden className="size-4" />
            {t("customers.back")}
          </Link>
        }
      />
      {customer.isPending ? (
        <Skeleton className="m-4 h-40" />
      ) : customer.isError ? (
        <p className="p-4 text-danger">{customer.error.message}</p>
      ) : c ? (
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-3 sm:p-4">
          {c.state === "suspended" ? (
            <Notice tone="danger">{t("detail.suspendedNotice", { when: c.suspended_at ? fullTime(c.suspended_at, i18n.language) : "" })}</Notice>
          ) : null}
          <Details customer={c} />
          <div className="flex flex-wrap gap-2">
            {canInvite(c) ? <Button onClick={() => setAction("invite")}>{t("detail.newInvite")}</Button> : null}
            {c.state === "active" ? (
              <Button variant="danger" onClick={() => setAction("suspend")}>
                {t("detail.suspend")}
              </Button>
            ) : (
              <Button variant="primary" onClick={() => setAction("restore")}>
                {t("detail.restore")}
              </Button>
            )}
          </div>
          {action === "suspend" ? <SuspendDialog customer={c} onClose={() => setAction(null)} /> : null}
          {action === "restore" ? <RestoreDialog customer={c} onClose={() => setAction(null)} /> : null}
          {action === "invite" ? <InviteDialog customer={c} onClose={() => setAction(null)} /> : null}
        </div>
      ) : null}
    </div>
  );
}

function Details({ customer: c }: { customer: Customer }) {
  const { t, i18n } = useTranslation("operator");
  const now = new Date();
  const when = (s?: string) => (s ? fullTime(s, i18n.language) : t("never"));
  const rows: [string, ReactNode][] = [
    [t("customers.slug"), <span className="font-mono">{c.slug}</span>],
    [t("customers.state"), <StateBadge state={c.state} />],
    [t("customers.owner"), <OwnerBadge customer={c} now={now} />],
    [t("detail.inviteExpires"), c.invite && !c.has_owner ? when(c.invite.expires_at) : "–"],
    [t("customers.users"), c.users],
    [t("customers.agents"), c.agents],
    [t("customers.hosts"), t("customers.hostsValue", { online: c.hosts_online, total: c.hosts })],
    [t("customers.storage"), <StorageText storage={c.storage} />],
    [t("customers.lastActivity"), when(c.last_activity_at)],
    [t("detail.created"), when(c.created_at)],
  ];
  if (c.suspended_reason) rows.push([t("detail.reason"), <span className="break-words">{c.suspended_reason}</span>]);
  return (
    <dl className="grid max-w-2xl grid-cols-1 gap-x-6 gap-y-2 rounded-lg border border-border bg-surface p-3 text-ui sm:grid-cols-[12rem_1fr]">
      {rows.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-fg-muted">{label}</dt>
          <dd className="tabular min-w-0 text-fg">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function SuspendDialog({ customer, onClose }: { customer: Customer; onClose: () => void }) {
  const { t } = useTranslation("operator");
  const suspend = useSuspendCustomer(customer.id);
  const [reason, setReason] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    suspend.mutate(reason.trim(), { onSuccess: onClose });
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("suspend.title", { name: customer.name })} closeLabel={t("common:close")}
      footer={
        <>
          <Button onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" form="operator-suspend" variant="danger" loading={suspend.isPending} disabled={!reason.trim()}>
            {t("detail.suspend")}
          </Button>
        </>
      }>
      <form id="operator-suspend" onSubmit={submit} className="flex flex-col gap-3 text-ui">
        <p>{t("suspend.body")}</p>
        <label className="flex flex-col gap-1">
          <span className="font-medium">{t("suspend.reason")}</span>
          <Textarea required maxLength={500} rows={3} value={reason} onChange={(e) => setReason(e.target.value)} />
          <span className="text-meta text-fg-subtle">{t("suspend.reasonHelp")}</span>
        </label>
        {suspend.error ? <p role="alert" className="text-danger">{suspend.error.message}</p> : null}
      </form>
    </Dialog>
  );
}

function RestoreDialog({ customer, onClose }: { customer: Customer; onClose: () => void }) {
  const { t } = useTranslation("operator");
  const restore = useRestoreCustomer(customer.id);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("restore.title", { name: customer.name })} closeLabel={t("common:close")}
      footer={
        <>
          <Button onClick={onClose}>{t("cancel")}</Button>
          <Button variant="primary" loading={restore.isPending} onClick={() => restore.mutate(undefined, { onSuccess: onClose })}>
            {t("detail.restore")}
          </Button>
        </>
      }>
      <p className="text-ui">{t("restore.body")}</p>
      {restore.error ? <p role="alert" className="text-ui text-danger">{restore.error.message}</p> : null}
    </Dialog>
  );
}

function InviteDialog({ customer, onClose }: { customer: Customer; onClose: () => void }) {
  const { t, i18n } = useTranslation("operator");
  const invite = useInviteOwner(customer.id);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("invite.title", { name: customer.name })} closeLabel={t("common:close")}
      footer={
        invite.data ? (
          <Button variant="primary" onClick={onClose}>{t("done")}</Button>
        ) : (
          <>
            <Button onClick={onClose}>{t("cancel")}</Button>
            <Button variant="primary" loading={invite.isPending} onClick={() => invite.mutate()}>
              {t("invite.create")}
            </Button>
          </>
        )
      }>
      <div className="flex flex-col gap-3 text-ui">
        {invite.data ? (
          <>
            <p>{t("invite.handOver", { expires: fullTime(invite.data.expires_at, i18n.language) })}</p>
            <SecretOnce secret={invite.data.url} />
          </>
        ) : (
          <p>{t("invite.body")}</p>
        )}
        {invite.error ? <p role="alert" className="text-danger">{invite.error.message}</p> : null}
      </div>
    </Dialog>
  );
}
