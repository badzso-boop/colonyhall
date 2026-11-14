// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/shared/ui";
import { useDeleteSaved, useSave, useSavedIndex } from "../api";
import { ReminderDialog } from "./ReminderDialog";

/**
 * The message menu's "Mentés" and "Emlékeztess…" (round 55, parts E and F),
 * for a feature that shows messages: whether it is saved, the toggle, and
 * the reminder dialog to render next to the menu.
 */
export function useSaveActions(messageId: string, enabled: boolean): { saved: boolean; toggle: () => void; remind: () => void; dialog: ReactNode } {
  const { t } = useTranslation("saved");
  const index = useSavedIndex(enabled);
  const save = useSave();
  const remove = useDeleteSaved();
  const [open, setOpen] = useState(false);
  const item = index.data?.get(messageId);
  const saved = Boolean(item && !item.done_at);
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  return {
    saved,
    toggle: () =>
      item && saved
        ? remove.mutate(item.id, { onSuccess: () => toast(t("removed")), onError: fail })
        : save.mutate({ message_id: messageId }, { onSuccess: () => toast(t("savedToast")), onError: fail }),
    remind: () => setOpen(true),
    dialog: open ? <ReminderDialog messageId={messageId} onClose={() => setOpen(false)} /> : null,
  };
}
