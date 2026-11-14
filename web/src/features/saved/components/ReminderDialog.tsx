import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useSession } from "@/features/identity";
import { fullTime } from "@/shared/lib";
import { Button, Dialog, Input, toast } from "@/shared/ui";
import { useSave, useUpdateSaved } from "../api";
import { fromLocalInput, reminderChoices, reminderTime, timeZoneOf } from "../model";

type Props = {
  /** The message to be reminded of; absent for a plain reminder (then a text is asked). */
  messageId?: string;
  /** An existing item whose reminder changes. */
  itemId?: string;
  onClose: () => void;
};

/**
 * "Emlékeztess…" (round 55, part E): in 20 minutes, in an hour, tomorrow
 * 9:00, next Monday 9:00, or a time picked, in the person's time zone.
 */
export function ReminderDialog({ messageId, itemId, onClose }: Props) {
  const { t, i18n } = useTranslation("saved");
  const session = useSession();
  const tz = timeZoneOf(session.data?.actor.timezone);
  const save = useSave();
  const update = useUpdateSaved();
  const [custom, setCustom] = useState("");
  const [note, setNote] = useState("");
  const needsNote = !messageId && !itemId;

  const run = (at: Date) => {
    const done = () => {
      toast(t("reminder.set", { when: fullTime(at.toISOString(), i18n.language) }));
      onClose();
    };
    const fail = (e: Error) => toast(e.message, { tone: "danger" });
    if (itemId) update.mutate({ id: itemId, remind_at: at.toISOString() }, { onSuccess: done, onError: fail });
    else save.mutate({ ...(messageId ? { message_id: messageId } : {}), remind_at: at.toISOString(), ...(note.trim() ? { note: note.trim() } : {}) }, { onSuccess: done, onError: fail });
  };
  const customAt = fromLocalInput(custom, tz);
  const blocked = needsNote && !note.trim();

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("reminder.title")}
      description={t("reminder.zone", { zone: tz })}
      closeLabel={t("common:close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" disabled={!customAt || blocked} loading={save.isPending || update.isPending} onClick={() => customAt && run(customAt)}>
            {t("reminder.setCustom")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {needsNote ? (
          <label className="flex flex-col gap-1">
            <span className="text-meta font-semibold text-fg-muted">{t("reminder.note")}</span>
            <Input autoFocus value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
          </label>
        ) : null}
        <div className="grid grid-cols-2 gap-2">
          {reminderChoices.map((c) => {
            const at = reminderTime(c, new Date(), tz);
            return (
              <Button key={c} variant="secondary" disabled={blocked} onClick={() => run(at)} className="h-auto flex-col items-start py-2">
                <span>{t(`reminder.${c}`)}</span>
                <span className="text-meta font-normal text-fg-muted">{fullTime(at.toISOString(), i18n.language)}</span>
              </Button>
            );
          })}
        </div>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("reminder.custom")}</span>
          <Input type="datetime-local" value={custom} onChange={(e) => setCustom(e.target.value)} />
        </label>
      </div>
    </Dialog>
  );
}
