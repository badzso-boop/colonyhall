// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useActors } from "@/features/identity";
import {
  Button,
  EmptyState,
  Notice,
  Select,
  Skeleton,
  toast,
} from "@/shared/ui";
import {
  useAddGrant,
  useConnectors,
  useExtensions,
  useGrants,
  useRevokeGrant,
} from "../api";

/** Which agent may use which connector. */
export function GrantsTab() {
  const { t } = useTranslation("workshop");
  const actors = useActors();
  const connectors = useConnectors();
  const extensions = useExtensions();
  const grants = useGrants();
  const add = useAddGrant();
  const revoke = useRevokeGrant();
  const [agent, setAgent] = useState("");
  const [target, setTarget] = useState("");
  if (actors.isPending || connectors.isPending || grants.isPending)
    return <Skeleton className="h-24 w-full" />;
  if (actors.isError || connectors.isError || grants.isError)
    return (
      <p className="text-ui text-danger">
        {(actors.error ?? connectors.error ?? grants.error)?.message}
      </p>
    );
  const agents = [...actors.data.values()]
    .filter((a) => a.kind === "agent" && !a.archived_at)
    .sort((a, b) => a.handle.localeCompare(b.handle));
  const conns = connectors.data.connectors;
  const exts = extensions.data?.extensions ?? [];
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  return (
    <div className="flex flex-col gap-3">
      <p className="text-ui text-fg-muted">{t("grants.intro")}</p>
      {agents.length === 0 ? (
        <Notice tone="info">{t("grants.noAgents")}</Notice>
      ) : null}
      {conns.length === 0 && exts.length === 0 ? (
        <Notice tone="info">{t("grants.noConnectors")}</Notice>
      ) : null}
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!agent || !target) return;
          const [kind, id] = target.split(":") as [
            "connector" | "extension",
            string,
          ];
          add.mutate(
            { agent_id: agent, target_kind: kind, target_id: id },
            { onError: fail },
          );
        }}
      >
        <label className="flex min-w-40 flex-1 flex-col gap-1 text-ui">
          <span className="font-medium">{t("grants.agent")}</span>
          <Select value={agent} onChange={(e) => setAgent(e.target.value)}>
            <option value="" />
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                @{a.handle} · {a.display_name}
              </option>
            ))}
          </Select>
        </label>
        <label className="flex min-w-40 flex-1 flex-col gap-1 text-ui">
          <span className="font-medium">{t("grants.target")}</span>
          <Select value={target} onChange={(e) => setTarget(e.target.value)}>
            <option value="" />
            {exts.length ? (
              <optgroup label={t("tabs.extensions")}>
                {exts.map((x) => (
                  <option key={x.id} value={`extension:${x.id}`}>
                    {x.title} (x_{x.slug})
                  </option>
                ))}
              </optgroup>
            ) : null}
            <optgroup label={t("tabs.connectors")}>
              {conns.map((c) => (
                <option key={c.id} value={`connector:${c.id}`}>
                  {c.title} ({c.slug})
                </option>
              ))}
            </optgroup>
          </Select>
        </label>
        <Button
          type="submit"
          variant="primary"
          loading={add.isPending}
          disabled={!agent || !target}
        >
          {t("grants.add")}
        </Button>
      </form>
      {grants.data.length === 0 ? (
        <EmptyState title={t("grants.empty")} />
      ) : (
        <ul className="flex flex-col divide-y divide-border rounded-lg border border-border bg-surface">
          {grants.data.map((g) => (
            <li
              key={g.id}
              className="flex flex-wrap items-center gap-2 px-3 py-2 text-ui"
            >
              <span className="min-w-0 flex-1">
                <span className="font-medium">
                  @{g.agent_handle ?? g.agent_id}
                </span>
                <span className="mx-2 text-fg-subtle">→</span>
                <code className="font-mono text-meta">
                  {g.target_name ?? g.target_kind}
                </code>
              </span>
              <Button
                size="sm"
                variant="ghost"
                disabled={revoke.isPending}
                onClick={() => revoke.mutate(g.id, { onError: fail })}
              >
                {t("grants.revoke")}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
