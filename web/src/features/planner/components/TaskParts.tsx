import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDown, ArrowUp, Plus } from "lucide-react";
import { cn } from "@/shared/lib";
import { Badge, IconButton, Input } from "@/shared/ui";
import { useCreateTask, useUpdateTask } from "../api";
import { firstLine, neighbours, nextStatus, type Task } from "../tasks";

export function QuickAdd() {
  const { t } = useTranslation("planner");
  const create = useCreateTask();
  const [title, setTitle] = useState("");
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const v = title.trim();
    if (!v) return;
    create.mutate({ title: v }, { onSuccess: () => setTitle("") });
  };
  return (
    <form onSubmit={submit} className="flex items-center gap-2">
      <Plus aria-hidden className="size-4 shrink-0 text-fg-subtle" />
      <Input aria-label={t("tasks.add")} placeholder={t("tasks.add")} value={title} onChange={(e) => setTitle(e.target.value)} maxLength={300} />
    </form>
  );
}

const tone = { started: "warning", scheduled: "neutral", done: "success" } as const;

/** The status circle: half for started, a tick for done; click cycles. */
export function StatusToggle({ task }: { task: Task }) {
  const { t } = useTranslation("planner");
  const update = useUpdateTask();
  const next = nextStatus(task.status);
  return (
    <button
      type="button"
      aria-label={t("tasks.cycle", { status: t(`tasks.status.${next}`) })}
      title={t(`tasks.status.${task.status}`)}
      onClick={() => update.mutate({ id: task.id, status: next })}
      className={cn(
        "mt-0.5 size-4 shrink-0 rounded-full border-[1.75px]",
        task.status === "scheduled" && "border-fg-subtle",
        task.status === "started" && "border-accent bg-[conic-gradient(var(--color-accent)_0_50%,transparent_50%_100%)]",
        task.status === "done" && "border-success bg-success shadow-[inset_0_0_0_3px_var(--color-surface-raised)]",
      )}
    />
  );
}

/** A task in a list: status toggle, title, first line; ↑/↓ for the keyboard and phones. */
export function TaskRow({ task, list, index, onOpen, compact }: { task: Task; list: Task[]; index: number; onOpen: (t: Task) => void; compact?: boolean }) {
  const { t } = useTranslation("planner");
  const update = useUpdateTask();
  const move = (to: number) => update.mutate({ id: task.id, move: true, ...neighbours(list, task.id, to) });
  return (
    <li className="group flex items-start gap-2 rounded-md px-2 py-1.5 hover:bg-surface-sunken">
      <StatusToggle task={task} />
      <button type="button" onClick={() => onOpen(task)} className="min-w-0 flex-1 text-left">
        <span className={cn("block truncate font-medium", task.status === "done" && "text-fg-subtle line-through")}>{task.title}</span>
        {!compact && firstLine(task.description) ? <span className="block truncate text-meta text-fg-subtle">{firstLine(task.description)}</span> : null}
      </button>
      {!compact ? <Badge tone={tone[task.status]}>{t(`tasks.status.${task.status}`)}</Badge> : null}
      <span className="flex opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 max-md:opacity-100">
        <IconButton size="sm" label={t("tasks.up")} icon={<ArrowUp className="size-3.5" />} disabled={index === 0} onClick={() => move(index - 1)} />
        <IconButton size="sm" label={t("tasks.down")} icon={<ArrowDown className="size-3.5" />} disabled={index === list.length - 1} onClick={() => move(index + 1)} />
      </span>
    </li>
  );
}
