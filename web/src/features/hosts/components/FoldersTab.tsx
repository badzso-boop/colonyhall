// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { FolderOpen, HardDrive } from "lucide-react";
import { useActors } from "@/features/identity";
import { Button, EmptyState } from "@/shared/ui";
import type { Host } from "../api";
import { grantsByPath } from "../model";
import { rootOf } from "../picker";
import { GrantStacks } from "./bits";
import { FolderPicker } from "./FolderPicker";
import { Mono } from "./OverviewTab";

/**
 * Mappák: the guard.yaml roots and every granted folder with the agents
 * that may read or write there; the folder picker browses the machine live
 * with the same badges.
 */
export function FoldersTab({ host }: { host: Host }) {
  const { t } = useTranslation("hosts");
  const actors = useActors();
  const [browsing, setBrowsing] = useState(false);
  const grants = useMemo(() => grantsByPath(host), [host]);
  const roots = host.report.roots;
  // Group the granted folders under their root; folders outside every root come last.
  const groups = useMemo(() => {
    const out = new Map<string, string[]>(roots.map((r) => [r, []]));
    const outside: string[] = [];
    for (const path of grants.keys()) {
      const root = rootOf(path, roots);
      if (root) out.get(root)!.push(path);
      else outside.push(path);
    }
    return { byRoot: [...out.entries()], outside };
  }, [grants, roots]);

  const badge = (path: string) => {
    const holders = grants.get(path);
    return holders ? <GrantStacks holders={holders} actors={actors.data} hostColor={host.color} /> : null;
  };
  const line = (path: string, label?: string) => (
    <li key={path} className="flex flex-wrap items-center justify-between gap-2 py-1.5">
      <Mono>{label ?? path}</Mono>
      {badge(path)}
    </li>
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <p className="mr-auto text-ui text-fg-muted">{t("folders.hint")}</p>
        <Button size="sm" icon={<FolderOpen className="size-4" />} onClick={() => setBrowsing(true)}>
          {t("folders.browse")}
        </Button>
      </div>
      {!roots.length && !grants.size ? <EmptyState icon={<HardDrive className="size-8" />} title={t("folders.empty")} /> : null}
      {groups.byRoot.map(([root, paths]) => (
        <section key={root} className="rounded-md border border-border px-3 py-2">
          <h3 className="flex items-center gap-2 text-ui font-semibold">
            <HardDrive aria-hidden className="size-4 text-fg-subtle" />
            <Mono>{root}</Mono>
          </h3>
          <ul className="mt-1 divide-y divide-border">
            {paths.map((p) => line(p, p === root ? t("folders.wholeRoot") : p.slice(root.length).replace(/^\//, "") || p))}
            {!paths.length ? <li className="py-1.5 text-ui text-fg-muted">{t("folders.noGrants")}</li> : null}
          </ul>
        </section>
      ))}
      {groups.outside.length ? (
        <section className="rounded-md border border-warning/40 px-3 py-2">
          <h3 className="text-ui font-semibold text-warning">{t("folders.outside")}</h3>
          <ul className="mt-1 divide-y divide-border">{groups.outside.map((p) => line(p))}</ul>
        </section>
      ) : null}
      <FolderPicker hostId={host.id} hostLastSeen={host.last_seen_at} open={browsing} onOpenChange={setBrowsing} mode="browse" badge={badge} />
    </div>
  );
}
