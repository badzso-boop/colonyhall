// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Package, Plus } from "lucide-react";
import { ApiError } from "@/shared/api";
import { localeOf } from "@/shared/i18n";
import { formatBytes } from "@/shared/lib";
import { Badge, Button, Checkbox, Dialog, EmptyState, Input, PageHeader, Skeleton } from "@/shared/ui";
import { useCreatePlan, usePlans, useUpdatePlan, type Plan } from "../api";
import { bytesToGb, gbToBytes, parseCores } from "../model";

/** Csomagok (round 51b): the plans the operator edits; storage is enforced, CPU and memory are what a hosted guard will get. */
export default function PlansPage() {
  const { t, i18n } = useTranslation("operator");
  const locale = localeOf(i18n.language);
  const plans = usePlans();
  const [editing, setEditing] = useState<Plan | "new" | null>(null);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        icon={<Package className="size-5" />}
        title={t("plans.title")}
        actions={
          <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => setEditing("new")}>
            {t("plans.new")}
          </Button>
        }
      />
      <p className="px-4 pt-3 text-ui text-fg-muted">{t("plans.body")}</p>
      {plans.isPending ? (
        <Skeleton className="m-4 h-32" />
      ) : plans.isError ? (
        <p className="p-4 text-danger">{plans.error.message}</p>
      ) : !plans.data.length ? (
        <EmptyState icon={<Package className="size-6" />} title={t("plans.empty")} />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto p-3 sm:p-4">
          <table className="w-full min-w-[40rem] text-ui">
            <caption className="sr-only">{t("plans.title")}</caption>
            <thead className="border-b border-border text-left text-meta text-fg-subtle">
              <tr>
                <th scope="col" className="px-2 py-1.5 font-medium">{t("plans.name")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("plans.storage")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("plans.cpu")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("plans.memory")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("plans.calls")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("plans.workspaces")}</th>
                <th scope="col" className="px-2 py-1.5"><span className="sr-only">{t("plans.edit")}</span></th>
              </tr>
            </thead>
            <tbody>
              {plans.data.map((p) => (
                <tr key={p.id} className="border-b border-border/60 last:border-0">
                  <td className="px-2 py-2 font-medium">
                    {p.name} {p.archived_at ? <Badge>{t("plans.archived")}</Badge> : null}
                  </td>
                  <td className="tabular px-2 text-right">{formatBytes(p.storage_bytes, locale)}</td>
                  <td className="tabular px-2 text-right">{p.cpu_cores}</td>
                  <td className="tabular px-2 text-right">{formatBytes(p.memory_bytes, locale)}</td>
                  <td className="tabular px-2 text-right">{p.ext_calls_month ? p.ext_calls_month.toLocaleString(locale) : t("plans.unlimited")}</td>
                  <td className="tabular px-2 text-right">{p.workspaces}</td>
                  <td className="px-2 text-right">
                    <Button size="sm" variant="ghost" onClick={() => setEditing(p)}>
                      {t("plans.edit")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing ? <PlanDialog plan={editing === "new" ? null : editing} onClose={() => setEditing(null)} /> : null}
    </div>
  );
}

function PlanDialog({ plan, onClose }: { plan: Plan | null; onClose: () => void }) {
  const { t } = useTranslation("operator");
  const create = useCreatePlan();
  const update = useUpdatePlan(plan?.id ?? "");
  const [name, setName] = useState(plan?.name ?? "");
  const [storage, setStorage] = useState(plan ? bytesToGb(plan.storage_bytes) : "");
  const [cpu, setCpu] = useState(plan ? String(plan.cpu_cores) : "");
  const [memory, setMemory] = useState(plan ? bytesToGb(plan.memory_bytes) : "");
  const [calls, setCalls] = useState(plan?.ext_calls_month ? String(plan.ext_calls_month) : "");
  const [archived, setArchived] = useState(!!plan?.archived_at);
  const [confirm, setConfirm] = useState(false);
  const storageBytes = gbToBytes(storage);
  const memoryBytes = gbToBytes(memory);
  const cores = parseCores(cpu);
  // Round 56b: the Workshop's monthly call budget; empty or 0 is unlimited.
  const callsN = calls.trim() === "" ? 0 : /^\d{1,9}$/.test(calls.trim()) ? Number(calls.trim()) : null;
  const ready = name.trim() !== "" && storageBytes !== null && memoryBytes !== null && cores !== null && callsN !== null;
  const mutation = plan ? update : create;
  const overQuota = mutation.error instanceof ApiError && mutation.error.status === 409 && mutation.error.problem?.type === "plan_over_quota";
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) return;
    const body = { name: name.trim(), storage_bytes: storageBytes, cpu_cores: cores, memory_bytes: memoryBytes, ext_calls_month: callsN ?? 0 };
    if (plan) update.mutate({ ...body, archived, confirm_over_quota: confirm }, { onSuccess: onClose });
    else create.mutate(body, { onSuccess: onClose });
  };
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={plan ? t("plans.editTitle", { name: plan.name }) : t("plans.new")} closeLabel={t("common:close")}
      footer={
        <>
          <Button onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" form="operator-plan" variant="primary" loading={mutation.isPending} disabled={!ready || (overQuota && !confirm)}>
            {t("plans.save")}
          </Button>
        </>
      }>
      <form id="operator-plan" onSubmit={submit} className="flex flex-col gap-3 text-ui">
        <label className="flex flex-col gap-1">
          <span className="font-medium">{t("plans.name")}</span>
          <Input required maxLength={60} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <label className="flex flex-col gap-1">
            <span className="font-medium">{t("plans.storageGb")}</span>
            <Input inputMode="decimal" value={storage} aria-invalid={storage !== "" && storageBytes === null} onChange={(e) => setStorage(e.target.value)} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="font-medium">{t("plans.cpu")}</span>
            <Input inputMode="numeric" value={cpu} aria-invalid={cpu !== "" && cores === null} onChange={(e) => setCpu(e.target.value)} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="font-medium">{t("plans.memoryGb")}</span>
            <Input inputMode="decimal" value={memory} aria-invalid={memory !== "" && memoryBytes === null} onChange={(e) => setMemory(e.target.value)} />
          </label>
        </div>
        <p className="text-meta text-fg-subtle">{t("plans.computeHelp")}</p>
        <label className="flex flex-col gap-1">
          <span className="font-medium">{t("plans.callsMonth")}</span>
          <Input inputMode="numeric" value={calls} placeholder={t("plans.unlimited")} aria-invalid={callsN === null} onChange={(e) => setCalls(e.target.value)} />
          <span className="text-meta text-fg-subtle">{t("plans.callsHelp")}</span>
        </label>
        {plan ? (
          <label className="flex items-center gap-2">
            <Checkbox checked={archived} onCheckedChange={(v) => setArchived(v === true)} />
            <span>{t("plans.archive")}</span>
          </label>
        ) : null}
        {overQuota ? (
          <label role="alert" className="flex items-start gap-2 rounded-md border border-warning/40 bg-warning-soft p-2 text-warning">
            <Checkbox checked={confirm} onCheckedChange={(v) => setConfirm(v === true)} className="mt-0.5" />
            <span>
              {mutation.error?.message} {t("plans.confirmOverQuota")}
            </span>
          </label>
        ) : mutation.error ? (
          <p role="alert" className="text-danger">{mutation.error.message}</p>
        ) : null}
      </form>
    </Dialog>
  );
}
