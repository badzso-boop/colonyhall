// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState } from "react";
import { useTasks } from "../api";
import { groupTasks } from "../tasks";
import { QuickAdd, TaskRow } from "./TaskParts";
import { TaskSheet } from "./TaskSheet";

/** The floating window's compact list: started first, then scheduled; quick add on top. */
export function TasksWindow() {
  const tasks = useTasks();
  const groups = useMemo(() => groupTasks(tasks.data ?? []), [tasks.data]);
  const [openId, setOpenId] = useState<string | null>(null);
  const open = tasks.data?.find((x) => x.id === openId);
  return (
    <div className="flex flex-col gap-2 p-2.5 text-ui">
      <QuickAdd />
      {(["started", "scheduled"] as const).map((s) => (
        <ul key={s}>
          {groups[s].map((task, i) => (
            <TaskRow key={task.id} task={task} list={groups[s]} index={i} onOpen={(x) => setOpenId(x.id)} compact />
          ))}
        </ul>
      ))}
      {open ? <TaskSheet task={open} onClose={() => setOpenId(null)} /> : null}
    </div>
  );
}
