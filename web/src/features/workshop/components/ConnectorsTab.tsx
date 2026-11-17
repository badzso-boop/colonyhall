// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Pencil, Plug, Plus, RefreshCw, Trash2 } from "lucide-react";
import { fullTime } from "@/shared/lib";
import {
  Badge,
  Button,
  Dialog,
  EmptyState,
  IconButton,
  Notice,
  Select,
  Skeleton,
  Switch,
  toast,
} from "@/shared/ui";
import {
  useConnectors,
  useDeleteConnector,
  useRefreshConnector,
  useSetApprovalOff,
  useSetToolRisk,
  type Connector,
  type ToolRisk,
} from "../api";
import { ConnectorDialog } from "./ConnectorDialog";

const riskTone = {
  read: "neutral",
  write: "warning",
  external: "danger",
} as const;

/** The connectors with their tools, risks and the approval switch. */
export function ConnectorsTab({ owner }: { owner: boolean }) {
  const { t } = useTranslation("workshop");
  const list = useConnectors();
  const [editing, setEditing] = useState<Connector | "new" | null>(null);
  const [deleting, setDeleting] = useState<Connector | null>(null);
  if (list.isPending) return <Skeleton className="h-24 w-full" />;
  if (list.isError)
    return <p className="text-ui text-danger">{list.error.message}</p>;
  const { connectors, secrets_available: keys } = list.data;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <span className="mr-auto" />
        <Button
          size="sm"
          variant="primary"
          icon={<Plus className="size-4" />}
          onClick={() => setEditing("new")}
        >
          {t("connectors.new")}
        </Button>
      </div>
      {!keys ? <Notice tone="warning">{t("connectors.noKeys")}</Notice> : null}
      {connectors.length === 0 ? (
        <EmptyState
          icon={<Plug className="size-5" />}
          title={t("connectors.empty")}
          body={t("connectors.emptyBody")}
        />
      ) : (
        <ul className="flex flex-col gap-3">
          {connectors.map((c) => (
            <ConnectorCard
              key={c.id}
              c={c}
              owner={owner}
              onEdit={() => setEditing(c)}
              onDelete={() => setDeleting(c)}
            />
          ))}
        </ul>
      )}
      {editing ? (
        <ConnectorDialog
          connector={editing === "new" ? null : editing}
          secretsAvailable={keys}
          onClose={() => setEditing(null)}
        />
      ) : null}
      {deleting ? (
        <DeleteDialog c={deleting} onClose={() => setDeleting(null)} />
      ) : null}
    </div>
  );
}

function ConnectorCard({
  c,
  owner,
  onEdit,
  onDelete,
}: {
  c: Connector;
  owner: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t, i18n } = useTranslation("workshop");
  const refresh = useRefreshConnector();
  const approval = useSetApprovalOff();
  const setRisk = useSetToolRisk();
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  return (
    <li className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-3">
      <div className="flex flex-wrap items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold">{c.title}</span>
            <code className="font-mono text-meta text-fg-subtle">{c.slug}</code>
            <Badge tone="neutral">{t(`connectors.kind.${c.kind}`)}</Badge>
            <Badge
              tone={
                c.state === "error"
                  ? "danger"
                  : c.state === "ok"
                    ? "success"
                    : "neutral"
              }
            >
              {t(`connectors.state.${c.state}`)}
            </Badge>
            {c.approval_off ? (
              <Badge tone="warning">{t("connectors.approvalOff")}</Badge>
            ) : null}
          </div>
          <p className="mt-1 truncate font-mono text-meta text-fg-muted">
            {c.base_url}
          </p>
          <p className="text-meta text-fg-muted">
            {c.has_secret
              ? c.secret_hint
                ? t("connectors.secret", { hint: c.secret_hint })
                : t("connectors.secretSet")
              : null}
            {c.tools_cached_at ? (
              <span className="ml-2">
                {t("connectors.fetched", {
                  when: fullTime(c.tools_cached_at, i18n.language),
                })}
              </span>
            ) : null}
          </p>
          {c.last_error ? <Notice tone="danger">{c.last_error}</Notice> : null}
        </div>
        <div className="flex gap-1">
          {c.kind === "mcp" ? (
            <IconButton
              label={t("connectors.refresh")}
              icon={<RefreshCw className="size-4" />}
              onClick={() => refresh.mutate(c.id, { onError: fail })}
              disabled={refresh.isPending}
            />
          ) : null}
          <IconButton
            label={t("connectors.edit")}
            icon={<Pencil className="size-4" />}
            onClick={onEdit}
          />
          <IconButton
            label={t("connectors.delete")}
            icon={<Trash2 className="size-4" />}
            onClick={onDelete}
          />
        </div>
      </div>

      <label className="flex items-start gap-3 text-ui">
        <Switch
          checked={c.approval_off}
          disabled={!owner || approval.isPending}
          onCheckedChange={(off) =>
            approval.mutate({ id: c.id, off }, { onError: fail })
          }
          aria-label={t("connectors.approvalLabel")}
        />
        <span className="flex flex-col">
          <span className="font-medium">{t("connectors.approvalLabel")}</span>
          <span className="text-meta text-fg-muted">
            {t("connectors.approvalHint")}
          </span>
        </span>
      </label>

      {c.tools.length === 0 ? (
        <p className="text-meta text-fg-muted">{t("connectors.noTools")}</p>
      ) : (
        <details>
          <summary className="cursor-pointer text-ui font-medium">
            {t("connectors.tools", { count: c.tools.length })}
          </summary>
          <p className="mt-1 text-meta text-fg-muted">
            {t("connectors.riskHint")}
          </p>
          <ul className="mt-2 flex flex-col divide-y divide-border rounded-md border border-border">
            {c.tools.map((tool) => (
              <li
                key={tool.name}
                className="flex flex-wrap items-center gap-2 px-2 py-1.5 text-ui"
              >
                <span className="min-w-0 flex-1 basis-48">
                  <code className="font-mono text-meta">
                    {c.slug}__{tool.name}
                  </code>
                  <span className="block truncate text-meta text-fg-muted">
                    {tool.description}
                  </span>
                </span>
                <Badge tone={riskTone[tool.effective_risk]}>
                  {t(`connectors.risk.${tool.effective_risk}`)}
                </Badge>
                {tool.overridden ? (
                  <span className="text-meta text-fg-subtle">
                    {t("connectors.overridden")}
                  </span>
                ) : null}
                <Select
                  aria-label={t("connectors.riskLabel")}
                  value={tool.effective_risk}
                  disabled={setRisk.isPending}
                  onChange={(e) => {
                    const risk = e.target.value as ToolRisk;
                    setRisk.mutate(
                      {
                        id: c.id,
                        tool: tool.name,
                        risk: risk === tool.risk ? "" : risk,
                      },
                      { onError: fail },
                    );
                  }}
                >
                  {(["read", "write", "external"] as const).map((r) => (
                    <option key={r} value={r}>
                      {t(`connectors.risk.${r}`)}
                    </option>
                  ))}
                </Select>
              </li>
            ))}
          </ul>
        </details>
      )}
    </li>
  );
}

function DeleteDialog({ c, onClose }: { c: Connector; onClose: () => void }) {
  const { t } = useTranslation("workshop");
  const del = useDeleteConnector();
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("connectors.deleteTitle", { name: c.title })}
      closeLabel={t("close")}
      footer={
        <Button
          variant="danger"
          loading={del.isPending}
          onClick={() =>
            del.mutate(c.id, {
              onSuccess: onClose,
              onError: (e) => toast(e.message, { tone: "danger" }),
            })
          }
        >
          {t("connectors.delete")}
        </Button>
      }
    >
      <p>{t("connectors.deleteBody")}</p>
    </Dialog>
  );
}
