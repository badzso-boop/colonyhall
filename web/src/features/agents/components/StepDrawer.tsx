// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Ban, Brain, ChevronRight, FilePen, FileText, Globe, PenLine, Puzzle, Search, Settings, TerminalSquare, type LucideIcon } from "lucide-react";
import { cn } from "@/shared/lib";
import type { Schemas } from "@/shared/api";
import { useReplyRun } from "../api";
import { formatDuration } from "../model";

type Step = Schemas["AgentStep"];

const icons: Record<Step["kind"], LucideIcon> = {
  read: FileText,
  write: FilePen,
  edit: FilePen,
  search: Search,
  command: TerminalSquare,
  web: Globe,
  mcp: Puzzle,
  think: Brain,
  answer: PenLine,
  denied: Ban,
  other: Settings,
};

/** The step list (paths are relative to the agent's folder, masked by the server). */
export function StepList({ steps }: { steps: Step[] }) {
  const { t } = useTranslation("agents");
  return (
    <ol className="flex flex-col gap-0.5 border-l-2 border-border py-0.5 pl-3">
      {steps.map((s, i) => {
        const Icon = icons[s.kind] ?? Settings;
        return (
          <li key={i} className={cn("flex min-w-0 items-center gap-1.5 text-meta", s.kind === "denied" ? "text-danger" : "text-fg-muted")}>
            <Icon aria-hidden className="size-3.5 shrink-0" />
            <span className="sr-only">{t(`steps.kind.${s.kind}`)}</span>
            <span className="min-w-0 flex-1 truncate" title={s.path ? `${s.label} · ${s.path}` : s.label}>
              {s.label}
              {s.path && !s.label.includes(s.path) ? <span className="ml-1 font-mono text-fg-subtle">{s.path}</span> : null}
            </span>
            {s.duration_ms ? <span className="tabular shrink-0 text-fg-subtle">{(s.duration_ms / 1000).toFixed(1)} s</span> : null}
          </li>
        );
      })}
    </ol>
  );
}

/**
 * Under an agent's answer (round 22): collapsed "N lépés · 3:12"; opened,
 * the steps of the run that wrote the answer. Humans only.
 */
export function StepDrawer({ messageId }: { messageId: string }) {
  const { t } = useTranslation("agents");
  const run = useReplyRun(messageId, true);
  const [open, setOpen] = useState(false);
  const listId = useId();
  const steps = run.data?.steps ?? [];
  if (!steps.length) return null;
  return (
    <div className="mt-1">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={listId}
        onClick={() => setOpen(!open)}
        className="inline-flex items-center gap-1 rounded-sm text-meta text-fg-muted hover:text-fg"
      >
        <ChevronRight aria-hidden className={cn("size-3.5 transition-transform motion-reduce:transition-none", open && "rotate-90")} />
        <span className="tabular">{t("steps.summary", { count: steps.length, time: formatDuration(run.data?.duration_ms ?? 0) })}</span>
      </button>
      {open ? (
        <div id={listId} className="mt-1">
          <StepList steps={steps} />
        </div>
      ) : null}
    </div>
  );
}
