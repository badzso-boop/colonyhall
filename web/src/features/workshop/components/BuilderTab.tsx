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
import { useAddGrant, useGrants, useRevokeGrant } from "../api";

/** The Builder (round 56d): which agents may build extensions, and what to tell them. */
export function BuilderTab() {
  const { t } = useTranslation("workshop");
  const actors = useActors();
  const grants = useGrants();
  const add = useAddGrant();
  const revoke = useRevokeGrant();
  const [agent, setAgent] = useState("");
  if (actors.isPending || grants.isPending)
    return <Skeleton className="h-24 w-full" />;
  if (actors.isError || grants.isError)
    return (
      <p className="text-ui text-danger">
        {(actors.error ?? grants.error)?.message}
      </p>
    );
  const all = [...actors.data.values()];
  const adminIDs = new Set(
    all
      .filter(
        (a) => a.kind === "human" && (a.role === "owner" || a.role === "admin"),
      )
      .map((a) => a.id),
  );
  const agents = all
    .filter(
      (a) =>
        a.kind === "agent" &&
        !a.archived_at &&
        a.owner_id &&
        adminIDs.has(a.owner_id),
    )
    .sort((a, b) => a.handle.localeCompare(b.handle));
  const builders = grants.data.filter((g) => g.target_kind === "workshop");
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  return (
    <div className="flex flex-col gap-3">
      <p className="text-ui text-fg-muted">{t("builder.intro")}</p>
      <Notice tone="info">{t("builder.safety")}</Notice>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (agent)
            add.mutate(
              { agent_id: agent, target_kind: "workshop" },
              { onError: fail, onSuccess: () => setAgent("") },
            );
        }}
      >
        <label className="flex min-w-48 flex-1 flex-col gap-1 text-ui">
          <span className="font-medium">{t("builder.agent")}</span>
          <Select value={agent} onChange={(e) => setAgent(e.target.value)}>
            <option value="" />
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                @{a.handle} · {a.display_name}
              </option>
            ))}
          </Select>
          <span className="text-meta text-fg-muted">
            {t("builder.agentHint")}
          </span>
        </label>
        <Button
          type="submit"
          variant="primary"
          disabled={!agent}
          loading={add.isPending}
        >
          {t("builder.make")}
        </Button>
      </form>
      {builders.length === 0 ? (
        <EmptyState title={t("builder.empty")} />
      ) : (
        <ul className="flex flex-col divide-y divide-border rounded-lg border border-border bg-surface">
          {builders.map((g) => (
            <li
              key={g.id}
              className="flex items-center gap-2 px-3 py-2 text-ui"
            >
              <span className="flex-1 font-medium">
                @{g.agent_handle ?? g.agent_id}
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
      <section className="flex flex-col gap-1">
        <h3 className="font-semibold">{t("builder.howTitle")}</h3>
        <p className="text-meta text-fg-muted">{t("builder.howHint")}</p>
        <pre className="rounded-md border border-border p-2 font-mono text-meta whitespace-pre-wrap select-all">
          {t("builder.instructions")}
        </pre>
      </section>
    </div>
  );
}
