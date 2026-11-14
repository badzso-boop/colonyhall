import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { useQueryClient } from "@tanstack/react-query";
import { commandUsage, fullTime, type Command } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { toast } from "@/shared/ui";
import { EmergencyStopDialog } from "@/features/admin";
import { StopAgentDialog } from "@/features/agents";
import { useSession, useUpdateProfile } from "@/features/identity";
import { ReminderDialog, timeZoneOf, useSave } from "@/features/saved";
import { searchPath } from "@/features/search";
import { conversationKeys, useMute, useNotifyLevel, useSetPin, type Message, type MessagePages } from "./api";
import { latestMessage, remindAt } from "./slash";

/** What the composer does with its text after a command: clear it, keep it, or show every command. */
export type SlashResult = "clear" | "keep" | "help";

type Dialog = { kind: "remind"; messageId: string } | { kind: "stop"; handle: string } | { kind: "emergency" } | null;

/**
 * Runs the composer's slash commands (round 55, part B) with the existing
 * APIs. "This message" is the thread's root in a thread composer and the
 * conversation's newest message otherwise; /pin in a conversation pins the
 * person's own newest message there. /stop and /emergency only open their
 * confirmation; a person who is not an admin gets "unknown command".
 */
export function useSlash({ channelId, threadRootId }: { channelId: string; threadRootId?: string }) {
  const { t, i18n } = useTranslation("conversation");
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [, dispatch] = usePanes();
  const session = useSession();
  const me = session.data?.actor;
  const admin = me?.role === "owner" || me?.role === "admin";
  const setPin = useSetPin();
  const save = useSave();
  const notify = useNotifyLevel(channelId);
  const mute = useMute(channelId);
  const profile = useUpdateProfile();
  const [dialog, setDialog] = useState<Dialog>(null);

  const thisMessage = (own: boolean): Message | undefined => {
    if (threadRootId) return qc.getQueryData<{ root: Message }>(conversationKeys.thread(threadRootId))?.root;
    return latestMessage(qc.getQueryData<MessagePages>(conversationKeys.messages(channelId)), (m) => !own || m.author_id === me?.id);
  };
  const fail = (e: unknown) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" });

  const run = async (cmd: Command): Promise<SlashResult> => {
    switch (cmd.kind) {
      case "unknown":
        toast(t("slash.unknown", { name: cmd.name }), { tone: "danger" });
        return "keep";
      case "invalid":
        toast(t("slash.usage", { usage: commandUsage(cmd.name) }), { tone: "danger" });
        return "keep";
      case "help":
        return "help";
      case "stop":
      case "emergency":
        if (!admin) {
          toast(t("slash.unknown", { name: cmd.kind }), { tone: "danger" });
          return "keep";
        }
        setDialog(cmd.kind === "stop" ? { kind: "stop", handle: cmd.handle } : { kind: "emergency" });
        return "clear";
      case "pin": {
        const m = thisMessage(!threadRootId);
        if (!m) break;
        if (m.pinned) {
          toast(t("slash.alreadyPinned"));
          return "clear";
        }
        await setPin.mutateAsync({ messageId: m.id, pinned: true });
        toast(t("slash.pinned"));
        return "clear";
      }
      case "save": {
        const m = thisMessage(false);
        if (!m) break;
        await save.mutateAsync({ message_id: m.id });
        toast(t("slash.saved"));
        return "clear";
      }
      case "remindThis": {
        const m = thisMessage(false);
        if (!m) break;
        setDialog({ kind: "remind", messageId: m.id });
        return "clear";
      }
      case "remind": {
        const at = remindAt(cmd.when, new Date(), timeZoneOf(me?.timezone));
        await save.mutateAsync({ remind_at: at.toISOString(), note: cmd.text.slice(0, 500) });
        toast(t("slash.reminder", { when: fullTime(at.toISOString(), i18n.language) }));
        return "clear";
      }
      case "mute":
        await notify.mutateAsync("none");
        toast(t("slash.notify", { level: t("notify.none") }));
        return "clear";
      case "unmute":
        await mute.mutateAsync("off");
        toast(t("slash.unmuted"));
        return "clear";
      case "notify":
        await notify.mutateAsync(cmd.level);
        toast(t("slash.notify", { level: t(`notify.${cmd.level}`) }));
        return "clear";
      case "status":
        if (!me) return "keep";
        await profile.mutateAsync({ actorId: me.id, body: { status_text: cmd.text.slice(0, 80) } });
        toast(t(cmd.text ? "slash.status" : "slash.statusCleared"));
        return "clear";
      case "search":
        void navigate(searchPath(cmd.query));
        return "clear";
      case "thread": {
        if (threadRootId) return "clear";
        const m = thisMessage(false);
        if (!m) break;
        dispatch({ type: "openThread", rootId: m.thread_root_id ?? m.id });
        return "clear";
      }
      case "open":
        void navigate(`/${cmd.view}`);
        return "clear";
      default:
        return "keep";
    }
    toast(t("slash.noMessage"), { tone: "danger" });
    return "keep";
  };

  const close = () => setDialog(null);
  let node: ReactNode = null;
  if (dialog?.kind === "remind") node = <ReminderDialog messageId={dialog.messageId} onClose={close} />;
  else if (dialog?.kind === "stop") node = <StopAgentDialog handle={dialog.handle} onClose={close} />;
  else if (dialog?.kind === "emergency") node = <EmergencyStopDialog onClose={close} />;

  return { run: (cmd: Command) => run(cmd).catch((e: unknown) => (fail(e), "keep" as const)), dialog: node, admin };
}
