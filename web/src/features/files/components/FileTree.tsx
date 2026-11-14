// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, File, Folder, Link2 } from "lucide-react";
import { cn } from "@/shared/lib";
import { Badge, Skeleton } from "@/shared/ui";
import { useDirectory } from "../api";
import type { GitLetter } from "../model";

const letterTone: Record<GitLetter, "warning" | "success" | "danger" | "neutral" | "info"> = { M: "warning", A: "success", D: "danger", "?": "neutral", R: "info" };

type TreeProps = {
  agentId: string;
  channelId: string;
  letters: Map<string, GitLetter>;
  dirty: Set<string>;
  selected: string | null;
  onOpen: (path: string) => void;
};

function Dir({ path, depth, ...p }: TreeProps & { path: string; depth: number }) {
  const { t } = useTranslation("files");
  const dir = useDirectory(p.agentId, p.channelId, path);
  const [open, setOpen] = useState<Set<string>>(new Set());
  if (dir.isPending) return <Skeleton className="my-1 h-4 w-1/2" />;
  if (!dir.data) return null;
  if (!dir.data.entries.length) return <p className="py-1 text-meta text-fg-subtle" style={{ paddingLeft: depth * 14 + 8 }}>{t("empty")}</p>;
  return (
    <ul role={depth ? "group" : "tree"} aria-label={depth ? undefined : t("title")}>
      {dir.data.entries.map((e) => {
        const full = path ? `${path}/${e.name}` : e.name;
        const isDir = e.kind === "dir";
        const expanded = open.has(full);
        const letter = p.letters.get(full);
        return (
          <li key={full} role="treeitem" aria-expanded={isDir ? expanded : undefined} aria-selected={p.selected === full}>
            <button
              type="button"
              onClick={() => {
                if (isDir) setOpen((s) => (s.has(full) ? new Set([...s].filter((x) => x !== full)) : new Set(s).add(full)));
                else p.onOpen(full);
              }}
              title={e.heavy ? t("heavy") : full}
              className={cn(
                "flex h-7 w-full items-center gap-1.5 rounded-sm pr-2 text-left font-mono text-meta hover:bg-surface-sunken",
                p.selected === full && "bg-accent-soft",
                e.heavy && "text-fg-subtle",
              )}
              style={{ paddingLeft: depth * 14 + 6 }}
            >
              {isDir ? (expanded ? <ChevronDown aria-hidden className="size-3.5 shrink-0" /> : <ChevronRight aria-hidden className="size-3.5 shrink-0" />) : <span className="w-3.5 shrink-0" />}
              {isDir ? <Folder aria-hidden className="size-3.5 shrink-0 text-fg-subtle" /> : e.kind === "symlink" ? <Link2 aria-hidden className="size-3.5 shrink-0 text-fg-subtle" /> : <File aria-hidden className="size-3.5 shrink-0 text-fg-subtle" />}
              <span className="min-w-0 flex-1 truncate">{e.name}</span>
              {isDir && p.dirty.has(full) ? <span aria-hidden className="size-1.5 rounded-full bg-warning" /> : null}
              {letter ? <Badge mono tone={letterTone[letter]} className="h-4 min-w-4 px-1">{letter}</Badge> : null}
            </button>
            {isDir && expanded && !e.heavy ? <Dir {...p} path={full} depth={depth + 1} /> : null}
          </li>
        );
      })}
      {dir.data.truncated ? <li className="py-1 text-meta text-fg-subtle" style={{ paddingLeft: depth * 14 + 8 }}>{t("truncated")}</li> : null}
    </ul>
  );
}

/** The agent's folder tree: lazy, git letters, a dot on changed folders, heavy folders collapsed. */
export function FileTree(p: TreeProps) {
  return <Dir {...p} path="" depth={0} />;
}
