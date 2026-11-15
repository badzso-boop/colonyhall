// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent, type ReactNode } from "react";
import { ApiError } from "@/shared/api";
import { localeOf } from "@/shared/i18n";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";
import { ArrowLeft, Building2 } from "lucide-react";
import { SecretOnce } from "@/features/admin";
import { formatBytes, fullTime } from "@/shared/lib";
import { Badge, Button, Checkbox, Dialog, Input, Notice, PageHeader, Select, Skeleton, Textarea, toast } from "@/shared/ui";
import {
  useAssignPlan,
  useBreakGlassList,
  useCloseOperatorBreakGlass,
  useCustomer,
  useInviteOwner,
  useOpenBreakGlass,
  useOperatorSession,
  usePlanCheck,
  usePlans,
  useRestoreCustomer,
  useSuspendCustomer,
} from "../api";
import { OwnerBadge, StateBadge, StorageText } from "../components/CustomerBadges";
import { breakGlassMaxMinutes, breakGlassDevMaxMinutes, breakGlassReady, canInvite, type Customer } from "../model";

type Action = "suspend" | "restore" | "invite" | "plan" | "breakGlass" | null;

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
            <Button onClick={() => setAction("plan")}>{t("plan.change")}</Button>
            {c.state === "active" ? <Button onClick={() => setAction("breakGlass")}>{t("breakGlass.open")}</Button> : null}
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
          {action === "plan" ? <PlanDialog customer={c} onClose={() => setAction(null)} /> : null}
          {action === "breakGlass" ? <BreakGlassDialog customer={c} onClose={() => setAction(null)} /> : null}
          <BreakGlassList customer={c} />
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
    [t("plan.label"), <PlanName id={c.plan_id} />],
    [t("breakGlass.label"), c.break_glass_open ? <Badge tone="warning">{t("breakGlass.isOpen")}</Badge> : "–"],
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

function PlanName({ id }: { id?: string }) {
  const { t, i18n } = useTranslation("operator");
  const plans = usePlans();
  if (!id) return <>{t("plan.none")}</>;
  const p = plans.data?.find((x) => x.id === id);
  if (!p) return <>…</>;
  return <>{t("plan.value", { name: p.name, storage: formatBytes(p.storage_bytes, localeOf(i18n.language)), cpu: p.cpu_cores })}</>;
}

/** Round 51b: the plan, with the check before it is assigned; one below the usage needs a confirmation. */
function PlanDialog({ customer, onClose }: { customer: Customer; onClose: () => void }) {
  const { t, i18n } = useTranslation("operator");
  const locale = localeOf(i18n.language);
  const plans = usePlans();
  const assign = useAssignPlan(customer.id);
  const [planId, setPlanId] = useState(customer.plan_id ?? "");
  const [confirm, setConfirm] = useState(false);
  const check = usePlanCheck(customer.id, planId);
  const chk = planId ? check.data : undefined;
  const full = chk ? !chk.fits : false;
  const choices = (plans.data ?? []).filter((p) => !p.archived_at || p.id === customer.plan_id);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    assign.mutate({ plan_id: planId || undefined, confirm_over_quota: confirm }, { onSuccess: onClose });
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("plan.title", { name: customer.name })} closeLabel={t("common:close")}
      footer={
        <>
          <Button onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" form="operator-assign-plan" variant={full ? "danger" : "primary"} loading={assign.isPending} disabled={full && !confirm}>
            {t("plan.assign")}
          </Button>
        </>
      }>
      <form id="operator-assign-plan" onSubmit={submit} className="flex flex-col gap-3 text-ui">
        <label className="flex flex-col gap-1">
          <span className="font-medium">{t("plan.label")}</span>
          <Select value={planId} onChange={(e) => { setPlanId(e.target.value); setConfirm(false); }}>
            <option value="">{t("plan.none")}</option>
            {choices.map((p) => (
              <option key={p.id} value={p.id}>
                {t("plan.value", { name: p.name, storage: formatBytes(p.storage_bytes, locale), cpu: p.cpu_cores })}
              </option>
            ))}
          </Select>
        </label>
        {chk ? (
          <p className={full ? "text-danger" : "text-fg-muted"}>
            {t(full ? "plan.wouldFill" : "plan.fits", { used: formatBytes(chk.used_bytes, locale), quota: formatBytes(chk.new_quota_bytes, locale) })}
          </p>
        ) : null}
        {full ? (
          <label className="flex items-start gap-2">
            <Checkbox checked={confirm} onCheckedChange={(v) => setConfirm(v === true)} className="mt-0.5" />
            <span>{t("plan.confirm")}</span>
          </label>
        ) : null}
        {assign.error ? <p role="alert" className="text-danger">{assign.error.message}</p> : null}
      </form>
    </Dialog>
  );
}

/** Round 51b: a read-only look into the workspace, with a reason and a time; the link (60 s, once) is shown once. */
function BreakGlassDialog({ customer, onClose }: { customer: Customer; onClose: () => void }) {
  const { t, i18n } = useTranslation("operator");
  const dev = !!useOperatorSession().data?.dev_access;
  const open = useOpenBreakGlass(customer.id);
  const [reason, setReason] = useState("");
  const [minutes, setMinutes] = useState("60");
  const m = Number(minutes);
  const ready = breakGlassReady(reason, m, dev);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (ready) open.mutate({ reason: reason.trim() || undefined, minutes: m });
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("breakGlass.title", { name: customer.name })} closeLabel={t("common:close")}
      footer={
        open.data ? (
          <Button variant="primary" onClick={onClose}>{t("done")}</Button>
        ) : (
          <>
            <Button onClick={onClose}>{t("cancel")}</Button>
            <Button type="submit" form="operator-break-glass" variant="danger" loading={open.isPending} disabled={!ready}>
              {t("breakGlass.open")}
            </Button>
          </>
        )
      }>
      {open.data ? (
        <div className="flex flex-col gap-3 text-ui">
          <p>{t("breakGlass.handOver", { until: fullTime(open.data.handoff_expires_at, i18n.language) })}</p>
          <a href={open.data.url} target="_blank" rel="noreferrer noopener" className="font-medium text-accent-text underline">
            {t("breakGlass.go")}
          </a>
        </div>
      ) : (
        <form id="operator-break-glass" onSubmit={submit} className="flex flex-col gap-3 text-ui">
          <p>{t("breakGlass.body")}</p>
          <label className="flex flex-col gap-1">
            <span className="font-medium">{t("breakGlass.reason")}</span>
            <Textarea required={!dev} maxLength={500} rows={3} value={reason} onChange={(e) => setReason(e.target.value)} />
            <span className="text-meta text-fg-subtle">{t(dev ? "breakGlass.reasonDev" : "breakGlass.reasonHelp")}</span>
          </label>
          <label className="flex flex-col gap-1">
            <span className="font-medium">{t("breakGlass.minutes")}</span>
            <Input inputMode="numeric" className="w-32" value={minutes} aria-invalid={minutes !== "" && !Number.isInteger(m)} onChange={(e) => setMinutes(e.target.value)} />
            <span className="text-meta text-fg-subtle">{t("breakGlass.minutesHelp", { max: dev ? breakGlassDevMaxMinutes : breakGlassMaxMinutes })}</span>
          </label>
          {open.error ? <p role="alert" className="text-danger">{open.error instanceof ApiError ? open.error.message : String(open.error)}</p> : null}
        </form>
      )}
    </Dialog>
  );
}

function BreakGlassList({ customer }: { customer: Customer }) {
  const { t, i18n } = useTranslation("operator");
  const list = useBreakGlassList(customer.id);
  const close = useCloseOperatorBreakGlass(customer.id);
  if (!list.data?.length) return null;
  return (
    <section className="flex max-w-2xl flex-col gap-2">
      <h2 className="text-ui font-semibold">{t("breakGlass.history")}</h2>
      <ul className="flex flex-col gap-2">
        {list.data.map((b) => (
          <li key={b.id} className="flex flex-col gap-1 rounded-lg border border-border bg-surface p-3 text-ui">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{b.operator_handle}</span>
              {b.open ? <Badge tone="warning">{t("breakGlass.isOpen")}</Badge> : <Badge>{t(`breakGlass.closedBy.${b.closed_by ?? "expired"}`)}</Badge>}
              {b.dev_mode ? <Badge tone="danger">{t("devAccess")}</Badge> : null}
              {b.open ? (
                <Button size="sm" variant="ghost" className="ml-auto" loading={close.isPending}
                  onClick={() => close.mutate(b.id, { onError: (e) => toast(e.message, { tone: "danger" }) })}>
                  {t("breakGlass.close")}
                </Button>
              ) : null}
            </div>
            <p className="break-words">{b.reason}</p>
            <p className="tabular text-meta text-fg-muted">
              {fullTime(b.opened_at, i18n.language)} – {fullTime(b.closed_at ?? b.expires_at, i18n.language)}
            </p>
          </li>
        ))}
      </ul>
    </section>
  );
}
