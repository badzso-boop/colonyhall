// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { NotebookPen, Pin, PinOff } from "lucide-react";
import { useActors } from "@/features/identity";
import { useNow } from "@/shared/hooks";
import { usePanes } from "@/shared/panes";
import { EmptyState, IconButton, Skeleton } from "@/shared/ui";
import { useActiveThread, useNotesOfThread, useSeen } from "../active";
import { NoteCard } from "./NoteCard";

/** The floating window: follows the open thread (or the pinned one) and shows its agents' notes. */
export function NotesWindow() {
  const { t } = useTranslation("notes");
  const { thread, pinned, pin } = useActiveThread();
  const [, dispatch] = usePanes();
  const actors = useActors();
  const now = useNow();
  const { notes, pending, root } = useNotesOfThread(thread);
  const mark = useSeen((s) => s.mark);
  const newest = notes[0]?.created_at;
  useEffect(() => {
    if (thread && newest) mark(thread, newest);
  }, [thread, newest, mark]);

  if (!thread) return <EmptyState icon={<NotebookPen className="size-8" />} title={t("openThreadHint")} body={t("openThreadBody")} />;
  return (
    <div className="flex flex-col gap-2 p-2.5 text-ui">
      <div className="flex items-center gap-1">
        <button type="button" onClick={() => dispatch({ type: "openThread", rootId: thread })} className="min-w-0 flex-1 truncate text-left font-semibold hover:underline" title={t("openThread")}>
          {root?.body.split("\n")[0] || t("untitled")}
        </button>
        {pinned ? (
          <IconButton size="sm" label={t("unpin")} icon={<PinOff className="size-3.5" />} onClick={() => pin(null)} />
        ) : (
          <IconButton size="sm" label={t("pin")} icon={<Pin className="size-3.5" />} onClick={() => pin(thread)} />
        )}
      </div>
      {pending ? <Skeleton className="h-16 w-full" /> : null}
      {!pending && notes.length === 0 ? <p className="text-meta text-fg-subtle">{t("noNotes")}</p> : null}
      {notes.map((n) => (
        <NoteCard key={n.id} note={n} agent={actors.data?.get(n.agent_id)} now={now} />
      ))}
    </div>
  );
}
