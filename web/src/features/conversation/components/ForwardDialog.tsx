import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Hash, Lock, MessageCircle } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { cn, fuzzyScore, messageHref, randomId } from "@/shared/lib";
import { Button, Dialog, Input, Notice, Textarea, toast } from "@/shared/ui";
import { useChannels, useForward, type Message } from "../api";
import { forwardTargets, needsLeakConfirm } from "../forward";
import { conversationTitle } from "../model";

/**
 * "Továbbítás" (round 55, part F): pick a conversation, add a comment
 * (which may mention), and confirm when the message leaves a private
 * channel or a DM. The server checks access on both sides.
 */
export function ForwardDialog({ message, onClose }: { message: Message; onClose: () => void }) {
  const { t } = useTranslation("conversation");
  const navigate = useNavigate();
  const channels = useChannels();
  const actors = useActors();
  const meId = useSession().data?.actor.id ?? "";
  const forward = useForward();
  const [query, setQuery] = useState("");
  const [target, setTarget] = useState<string | null>(null);
  const [comment, setComment] = useState("");
  const source = channels.data?.find((c) => c.id === message.channel_id);
  const options = useMemo(() => {
    const list = forwardTargets(channels.data ?? []).map((c) => ({ c, title: actors.data ? conversationTitle(c, actors.data, meId) : c.name }));
    if (!query.trim()) return list;
    return list
      .map((o) => ({ ...o, score: fuzzyScore(o.title, query) }))
      .filter((o) => o.score > 0)
      .sort((a, b) => b.score - a.score);
  }, [channels.data, actors.data, meId, query]);
  const leak = target !== null && needsLeakConfirm(source, target);
  const sourceTitle = source && actors.data ? conversationTitle(source, actors.data, meId) : "";

  const send = () => {
    if (!target) return;
    forward.mutate(
      { messageId: message.id, channel_id: target, client_msg_id: randomId(), ...(comment.trim() ? { comment: comment.trim() } : {}) },
      {
        onSuccess: (res) => {
          onClose();
          const m = res.message;
          toast(t("forward.done"), m ? { action: { label: t("forward.open"), run: () => navigate(messageHref(m)) } } : undefined);
        },
        onError: (e) => toast(e.message, { tone: "danger" }),
      },
    );
  };

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("forward.title")}
      closeLabel={t("new.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("new.cancel")}
          </Button>
          <Button variant="primary" disabled={!target} loading={forward.isPending} onClick={send}>
            {t(leak ? "forward.sendAnyway" : "forward.send")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <Input autoFocus placeholder={t("forward.search")} aria-label={t("forward.search")} value={query} onChange={(e) => setQuery(e.target.value)} />
        <ul role="listbox" aria-label={t("forward.targets")} className="max-h-56 overflow-y-auto rounded-md border border-border p-1">
          {options.map(({ c, title }) => (
            <li
              key={c.id}
              role="option"
              aria-selected={target === c.id}
              tabIndex={0}
              onClick={() => setTarget(c.id)}
              onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && (e.preventDefault(), setTarget(c.id))}
              className={cn("flex h-8 cursor-pointer items-center gap-2 rounded-sm px-2 text-ui outline-none focus-visible:bg-surface-sunken", target === c.id ? "bg-accent-soft font-semibold" : "hover:bg-surface-sunken")}
            >
              {c.kind === "dm" ? <MessageCircle aria-hidden className="size-4 text-fg-subtle" /> : c.kind === "private" ? <Lock aria-hidden className="size-4 text-fg-subtle" /> : <Hash aria-hidden className="size-4 text-fg-subtle" />}
              <span className="truncate">{title}</span>
            </li>
          ))}
        </ul>
        <Textarea rows={2} placeholder={t("forward.comment")} aria-label={t("forward.comment")} value={comment} onChange={(e) => setComment(e.target.value)} />
        {leak ? <Notice tone="warning">{t(source?.kind === "dm" ? "forward.leakDm" : "forward.leak", { name: sourceTitle })}</Notice> : null}
      </div>
    </Dialog>
  );
}
