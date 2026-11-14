// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState, type DragEvent } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, ListChecks } from "lucide-react";
import { cn } from "@/shared/lib";
import { useLocalStorage } from "@/shared/hooks";
import { EmptyState, PageHeader, Skeleton } from "@/shared/ui";
import { openWindow } from "@/shared/windows";
import { useTasks, useUpdateTask } from "../api";
import { QuickAdd, StatusToggle, TaskRow } from "../components/TaskParts";
import { TaskSheet } from "../components/TaskSheet";
import { AccessButton } from "../components/AccessDialog";
import { firstLine, groupTasks, neighbours, statusOrder, type Status, type Task } from "../tasks";

function ListView({ groups, onOpen }: { groups: Record<Status, Task[]>; onOpen: (t: Task) => void }) {
  const { t } = useTranslation("planner");
  const [showDone, setShowDone] = useState(false);
  return (
    <div className="flex flex-col gap-5">
      {statusOrder.map((s) => {
        const list = groups[s];
        const collapsed = s === "done" && !showDone;
        return (
          <section key={s}>
            <h2 className="mb-1 flex items-center gap-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">
              {s === "done" ? (
                <button type="button" aria-expanded={showDone} onClick={() => setShowDone(!showDone)} className="flex items-center gap-1">
                  {showDone ? <ChevronDown aria-hidden className="size-3.5" /> : <ChevronRight aria-hidden className="size-3.5" />}
                  {t(`tasks.groups.${s}`)} · {list.length}
                </button>
              ) : (
                <>
                  {t(`tasks.groups.${s}`)} · {list.length}
                </>
              )}
            </h2>
            {collapsed ? null : (
              <ul>
                {list.map((task, i) => (
                  <TaskRow key={task.id} task={task} list={list} index={i} onOpen={onOpen} />
                ))}
              </ul>
            )}
          </section>
        );
      })}
    </div>
  );
}

/** Three columns; dragging a card between columns sets its status, within a column its place. */
function BoardView({ groups, onOpen }: { groups: Record<Status, Task[]>; onOpen: (t: Task) => void }) {
  const { t } = useTranslation("planner");
  const update = useUpdateTask();
  const [over, setOver] = useState<Status | null>(null);
  const drop = (e: DragEvent, status: Status, index: number) => {
    e.preventDefault();
    e.stopPropagation();
    setOver(null);
    const id = e.dataTransfer.getData("text/x-task");
    const task = Object.values(groups).flat().find((x) => x.id === id);
    if (!task) return;
    update.mutate({ id, ...(task.status !== status ? { status } : {}), move: true, ...neighbours(groups[status], id, index) });
  };
  return (
    <div className="grid gap-3 md:grid-cols-3">
      {statusOrder.map((s) => (
        <section
          key={s}
          aria-label={t(`tasks.groups.${s}`)}
          onDragOver={(e) => {
            e.preventDefault();
            setOver(s);
          }}
          onDragLeave={() => setOver((o) => (o === s ? null : o))}
          onDrop={(e) => drop(e, s, groups[s].length)}
          className={cn("flex min-h-40 flex-col gap-2 rounded-lg border border-border bg-surface-sunken p-2", over === s && "border-accent")}
        >
          <h2 className="px-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">
            {t(`tasks.groups.${s}`)} · {groups[s].length}
          </h2>
          {groups[s].map((task, i) => (
            <article
              key={task.id}
              draggable
              onDragStart={(e) => e.dataTransfer.setData("text/x-task", task.id)}
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => drop(e, s, i)}
              className="flex cursor-grab items-start gap-2 rounded-md border border-border bg-surface p-2 shadow-raised active:cursor-grabbing"
            >
              <StatusToggle task={task} />
              <button type="button" onClick={() => onOpen(task)} className="min-w-0 flex-1 text-left">
                <span className={cn("block font-medium", s === "done" && "text-fg-subtle line-through")}>{task.title}</span>
                {firstLine(task.description) ? <span className="block truncate text-meta text-fg-subtle">{firstLine(task.description)}</span> : null}
              </button>
            </article>
          ))}
          {groups[s].length === 0 ? <p className="px-1 text-meta text-fg-subtle">{t("tasks.emptyGroup")}</p> : null}
        </section>
      ))}
    </div>
  );
}

/** Feladatok (round 32, part 3). */
export default function TasksPage() {
  const { t } = useTranslation("planner");
  const tasks = useTasks();
  const [view, setView] = useLocalStorage<"list" | "board">("colony.tasks.view", "list");
  const [openId, setOpenId] = useState<string | null>(null);
  const groups = useMemo(() => groupTasks(tasks.data ?? []), [tasks.data]);
  const open = tasks.data?.find((x) => x.id === openId);
  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<ListChecks className="size-5" />}
        title={t("tasks.title")}
        onPopOut={() => openWindow("tasks")}
        popOutLabel={t("tasks.popOut")}
        actions={
          <>
            <AccessButton kind="tasks" />
            <div role="radiogroup" aria-label={t("tasks.view")} className="flex rounded-md border border-border p-0.5">
              {(["list", "board"] as const).map((v) => (
                <button
                  key={v}
                  type="button"
                  role="radio"
                  aria-checked={view === v}
                  onClick={() => setView(v)}
                  className={cn("rounded-sm px-2.5 py-1 text-ui", view === v ? "bg-accent-soft font-semibold text-fg" : "text-fg-muted")}
                >
                  {t(`tasks.${v}`)}
                </button>
              ))}
            </div>
          </>
        }
      />
      <div className="mx-auto flex w-full max-w-5xl min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
        <QuickAdd />
        {tasks.isPending ? <Skeleton className="h-5 w-1/2" /> : null}
        {tasks.data && tasks.data.length === 0 ? <EmptyState icon={<ListChecks className="size-8" />} title={t("tasks.empty")} /> : null}
        {tasks.data?.length ? view === "list" ? <ListView groups={groups} onOpen={(x) => setOpenId(x.id)} /> : <BoardView groups={groups} onOpen={(x) => setOpenId(x.id)} /> : null}
      </div>
      {open ? <TaskSheet task={open} onClose={() => setOpenId(null)} /> : null}
    </div>
  );
}
