// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { fullTime } from "@/shared/lib";
import { Button, Dialog, Input, Markdown, Select, Sheet, Textarea } from "@/shared/ui";
import { useDeleteTask, useUpdateTask } from "../api";
import { statusOrder, type Task } from "../tasks";

/** A task's details: title, markdown description (rendered or edited), status, times; delete behind a confirmation. */
export function TaskSheet({ task, onClose }: { task: Task; onClose: () => void }) {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const update = useUpdateTask();
  const del = useDeleteTask();
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(task.title);
  const [desc, setDesc] = useState(task.description);
  const [confirm, setConfirm] = useState(false);

  return (
    <Sheet open onOpenChange={(o) => !o && onClose()} title={task.title} closeLabel={t("tasks.cancel")}>
      <div className="flex flex-col gap-4">
        <label className="flex flex-col gap-1 text-ui">
          {t("tasks.statusLabel")}
          <Select value={task.status} onChange={(e) => update.mutate({ id: task.id, status: e.target.value as Task["status"] })}>
            {statusOrder.map((s) => (
              <option key={s} value={s}>
                {t(`tasks.status.${s}`)}
              </option>
            ))}
          </Select>
        </label>
        {editing ? (
          <>
            <label className="flex flex-col gap-1 text-ui">
              {t("tasks.titleLabel")}
              <Input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={300} />
            </label>
            <label className="flex flex-col gap-1 text-ui">
              {t("tasks.description")}
              <Textarea value={desc} onChange={(e) => setDesc(e.target.value)} rows={10} className="font-mono text-ui" />
            </label>
            <div className="flex justify-end gap-2">
              <Button onClick={() => setEditing(false)}>{t("tasks.cancel")}</Button>
              <Button
                variant="primary"
                loading={update.isPending}
                onClick={() => update.mutate({ id: task.id, title: title.trim() || task.title, description: desc }, { onSuccess: () => setEditing(false) })}
              >
                {t("tasks.save")}
              </Button>
            </div>
          </>
        ) : (
          <>
            <div className="flex items-center justify-between">
              <h3 className="text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("tasks.description")}</h3>
              <Button size="sm" variant="ghost" onClick={() => setEditing(true)}>
                {t("tasks.edit")}
              </Button>
            </div>
            {task.description ? (
              <Markdown text={task.description} copyLabel={t("tasks.copy")} copiedLabel={t("tasks.copied")} />
            ) : (
              <p className="text-ui text-fg-muted">{t("tasks.noDescription")}</p>
            )}
          </>
        )}
        <div className="text-meta text-fg-subtle">
          {task.started_at ? <p>{t("tasks.startedAt", { when: fullTime(task.started_at, locale) })}</p> : null}
          {task.done_at ? <p>{t("tasks.doneAt", { when: fullTime(task.done_at, locale) })}</p> : null}
        </div>
        <Button variant="danger" className="self-start" onClick={() => setConfirm(true)}>
          {t("tasks.delete")}
        </Button>
      </div>
      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("tasks.deleteTitle")}
        description={t("tasks.deleteBody", { title: task.title })}
        closeLabel={t("tasks.cancel")}
        footer={
          <>
            <Button onClick={() => setConfirm(false)}>{t("tasks.cancel")}</Button>
            <Button variant="danger" loading={del.isPending} onClick={() => del.mutate(task.id, { onSuccess: onClose })}>
              {t("tasks.delete")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </Sheet>
  );
}
