// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { Fragment, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { ChevronDown, ChevronRight, Download, ScrollText, ShieldCheck, ShieldX } from "lucide-react";
import { useChannels } from "@/features/conversation";
import { useActors } from "@/features/identity";
import { cn, fullTime } from "@/shared/lib";
import { Avatar, Button, EmptyState, Input, Notice, Popover, PopoverContent, PopoverTrigger, Select, Skeleton, Checkbox } from "@/shared/ui";
import { useAuditActions, useAuditPages, useVerifyAudit, type AuditFilters } from "../api";
import { AdminPage } from "../components/AdminPage";
import { auditExportQuery, payloadRows, type AuditEntry } from "../model";

const targetTypes = ["actor", "channel", "message", "host", "agent", "run", "schedule", "workspace", "token", "file", "audit"];

/** Audit (/admin/audit): the hash-chained log, with filters, verification and a CSV export. */
export default function AuditPage() {
  const { t } = useTranslation("admin");
  const [filters, setFilters] = useState<AuditFilters>({});
  const exportHref = useMemo(() => {
    const q = auditExportQuery(filters);
    return `/api/v1/admin/audit/export${q ? `?${q}` : ""}`;
  }, [filters]);
  return (
    <AdminPage
      icon={<ScrollText className="size-5" />}
      title={t("audit.title")}
      actions={
        <a
          href={exportHref}
          download
          className="inline-flex h-7 items-center gap-1.5 rounded-md border border-border-strong bg-surface px-2.5 text-ui hover:bg-surface-sunken"
        >
          <Download aria-hidden className="size-4" />
          {t("audit.export")}
        </a>
      }
    >
      <AuditBody filters={filters} setFilters={setFilters} />
    </AdminPage>
  );
}

function AuditBody({ filters, setFilters }: { filters: AuditFilters; setFilters: (f: AuditFilters) => void }) {
  const { t } = useTranslation("admin");
  const actors = useActors();
  const pages = useAuditPages(filters);
  const verify = useVerifyAudit();
  const entries = pages.data?.pages.flatMap((p) => p.entries) ?? [];
  const brokenAt = verify.data && !verify.data.ok ? verify.data.broken_at : undefined;
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-3 sm:p-4">
      <div className="mb-3 flex flex-wrap items-end gap-2">
        <label className="flex flex-col gap-1 text-meta text-fg-subtle">
          {t("audit.actor")}
          <Select value={filters.actor_id ?? ""} onChange={(e) => setFilters({ ...filters, actor_id: e.target.value || undefined })} className="w-44">
            <option value="">{t("audit.all")}</option>
            {[...(actors.data?.values() ?? [])].map((a) => (
              <option key={a.id} value={a.id}>
                @{a.handle}
              </option>
            ))}
          </Select>
        </label>
        <ActionFilter value={filters.action ?? []} onChange={(action) => setFilters({ ...filters, action: action.length ? action : undefined })} />
        <label className="flex flex-col gap-1 text-meta text-fg-subtle">
          {t("audit.targetType")}
          <Select value={filters.target_type ?? ""} onChange={(e) => setFilters({ ...filters, target_type: e.target.value || undefined })} className="w-36">
            <option value="">{t("audit.all")}</option>
            {targetTypes.map((x) => (
              <option key={x} value={x}>
                {x}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-meta text-fg-subtle">
          {t("audit.from")}
          <Input type="date" onChange={(e) => setFilters({ ...filters, from: e.target.value ? new Date(`${e.target.value}T00:00:00`).toISOString() : undefined })} className="w-36" />
        </label>
        <label className="flex flex-col gap-1 text-meta text-fg-subtle">
          {t("audit.to")}
          <Input type="date" onChange={(e) => setFilters({ ...filters, to: e.target.value ? new Date(new Date(`${e.target.value}T00:00:00`).getTime() + 86_400_000).toISOString() : undefined })} className="w-36" />
        </label>
        <Button className="ml-auto" loading={verify.isPending} icon={<ShieldCheck className="size-4" />} onClick={() => verify.mutate()}>
          {t("audit.verify")}
        </Button>
      </div>
      {verify.error ? <Notice tone="danger">{verify.error.message}</Notice> : null}
      {verify.data ? (
        verify.data.ok ? (
          <Notice tone="success" icon={<ShieldCheck className="size-4" />}>
            {t("audit.verifyOk", { count: verify.data.checked })}
          </Notice>
        ) : (
          <Notice tone="danger" icon={<ShieldX className="size-4" />}>
            {t("audit.verifyBroken", { seq: verify.data.broken_at })}
          </Notice>
        )
      ) : null}
      {pages.isPending ? (
        <Skeleton className="mt-3 h-40 w-full" />
      ) : pages.isError ? (
        <p className="text-danger">{pages.error.message}</p>
      ) : entries.length ? (
        <div className="mt-3 overflow-x-auto">
          <table className="w-full min-w-[40rem] text-ui">
            <thead className="border-b border-border text-left text-meta text-fg-subtle">
              <tr>
                <th className="w-7" />
                <th className="px-2 py-1.5 font-medium">{t("audit.time")}</th>
                <th className="px-2 py-1.5 font-medium">{t("audit.actor")}</th>
                <th className="px-2 py-1.5 font-medium">{t("audit.action")}</th>
                <th className="px-2 py-1.5 font-medium">{t("audit.target")}</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <AuditRow key={e.seq} entry={e} broken={e.seq === brokenAt} />
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <EmptyState title={t("audit.empty")} />
      )}
      {pages.hasNextPage ? (
        <div className="mt-3 flex justify-center">
          <Button size="sm" variant="ghost" loading={pages.isFetchingNextPage} onClick={() => void pages.fetchNextPage()}>
            {t("more")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function ActionFilter({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const { t } = useTranslation("admin");
  const actions = useAuditActions();
  return (
    <div className="flex flex-col gap-1 text-meta text-fg-subtle">
      {t("audit.action")}
      <Popover>
        <PopoverTrigger asChild>
          <Button className="w-44 justify-between">
            <span className="truncate">{value.length ? t("audit.actionsPicked", { count: value.length }) : t("audit.all")}</span>
            <ChevronDown aria-hidden className="size-4 shrink-0" />
          </Button>
        </PopoverTrigger>
        <PopoverContent label={t("audit.action")}>
          <ul className="flex max-h-72 flex-col gap-1 overflow-y-auto">
            {(actions.data ?? []).map((a) => (
              <li key={a}>
                <label className="flex items-center gap-2 font-mono text-meta text-fg">
                  <Checkbox checked={value.includes(a)} onCheckedChange={(on) => onChange(on === true ? [...value, a] : value.filter((x) => x !== a))} />
                  {a}
                </label>
              </li>
            ))}
          </ul>
        </PopoverContent>
      </Popover>
    </div>
  );
}

function AuditRow({ entry: e, broken }: { entry: AuditEntry; broken: boolean }) {
  const { t, i18n } = useTranslation("admin");
  const actors = useActors();
  const channels = useChannels();
  const [open, setOpen] = useState(false);
  const actor = actors.data?.get(e.actor_id);
  const channel = e.target_type === "channel" ? channels.data?.find((c) => c.id === e.target_id) : undefined;
  const targetActor = e.target_type === "actor" ? actors.data?.get(e.target_id) : undefined;
  const rows = payloadRows(e.payload);
  return (
    <Fragment>
      <tr className={cn("border-b border-border/60", broken && "bg-danger-soft")}>
        <td className="pl-1">
          <button type="button" aria-expanded={open} aria-label={t("audit.details")} onClick={() => setOpen(!open)} className="grid size-6 place-items-center rounded-sm text-fg-subtle hover:bg-surface-sunken">
            {open ? <ChevronDown aria-hidden className="size-4" /> : <ChevronRight aria-hidden className="size-4" />}
          </button>
        </td>
        <td className="px-2 py-1.5 whitespace-nowrap text-fg-muted">{fullTime(e.at, i18n.language)}</td>
        <td className="px-2">
          <span className="flex items-center gap-1.5 whitespace-nowrap">
            {actor ? <Avatar kind={actor.kind === "agent" ? "agent" : "human"} name={actor.display_name || actor.handle} emoji={actor.avatar_emoji} size="sm" /> : null}
            {actor ? `@${actor.handle}` : t("audit.system")}
          </span>
        </td>
        <td className="px-2 font-mono text-meta">{e.action}</td>
        <td className="px-2 text-fg-muted">
          {channel ? <Link to={`/c/${channel.id}`} className="hover:underline">#{channel.name}</Link> : targetActor ? `@${targetActor.handle}` : <span className="text-meta">{e.target_type} · {e.target_id.slice(0, 8)}</span>}
        </td>
      </tr>
      {open ? (
        <tr className="border-b border-border/60 bg-surface-sunken">
          <td />
          <td colSpan={4} className="px-2 py-2">
            <p className="mb-1 text-meta text-fg-subtle">{t("audit.seq", { seq: e.seq })}</p>
            {rows.length ? (
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-meta">
                {rows.map((r) => (
                  <div key={r.key} className="contents">
                    <dt className="font-mono text-fg-subtle">{r.key}</dt>
                    <dd className="min-w-0 font-mono break-all">{r.value}</dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p className="text-meta text-fg-muted">{t("audit.noPayload")}</p>
            )}
          </td>
        </tr>
      ) : null}
    </Fragment>
  );
}
