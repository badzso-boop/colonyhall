import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";
import { Hash, Lock, SendHorizontal } from "lucide-react";
import { feedItems, fullTime, messageTime } from "@/shared/lib";
import { localeOf } from "@/shared/i18n";
import { Avatar, EmptyState, IconButton, Skeleton } from "@/shared/ui";
import { useActors, useSession } from "@/features/identity";
import { useChannels, useMarkRead, useMessages, useSendMessage } from "../api";
import { conversationTitle, mainStream } from "../model";

// The first version of the conversation view; round 31 part 5 adds markdown,
// thread summaries, the thread pane, attachments and history paging.
export default function ConversationPage() {
  const { channelId = "" } = useParams();
  const { t, i18n } = useTranslation("conversation");
  const locale = localeOf(i18n.language);
  const channels = useChannels();
  const actors = useActors();
  const session = useSession();
  const messages = useMessages(channelId);
  const send = useSendMessage(channelId);
  const markRead = useMarkRead();
  const [draft, setDraft] = useState("");
  const bottom = useRef<HTMLDivElement>(null);
  const meId = session.data?.actor.id ?? "";

  const channel = channels.data?.find((c) => c.id === channelId);
  const items = useMemo(
    () => (channel && messages.data ? feedItems(mainStream(messages.data, channel.kind), new Date(), locale, { today: t("common:today"), yesterday: t("common:yesterday") }) : []),
    [channel, messages.data, locale, t],
  );

  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "end" });
    if (channel && channel.unread > 0) markRead.mutate(channel.id);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mark read when new messages arrive
  }, [items.length, channelId]);

  if (channels.data && !channel) return <EmptyState title={t("notFound")} />;
  if (!channel || !actors.data) return <Skeleton className="m-4 h-6 w-1/3" />;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    const body = draft.trim();
    if (!body) return;
    send.mutate({ body, client_msg_id: crypto.randomUUID() }, { onSuccess: () => setDraft("") });
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-surface px-4">
        {channel.kind === "private" ? <Lock aria-hidden className="size-4 text-fg-subtle" /> : channel.kind === "public" ? <Hash aria-hidden className="size-4 text-fg-subtle" /> : null}
        <h1 className="text-title font-semibold">{conversationTitle(channel, actors.data, meId)}</h1>
        {channel.description ? <p className="min-w-0 flex-1 truncate text-ui text-fg-muted">{channel.description}</p> : null}
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto py-2">
        {items.map((it) =>
          it.separator ? (
            <div key={`d-${it.day}`} className="my-3 flex items-center gap-3 px-4 text-meta font-medium text-fg-subtle before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border">
              {it.label}
            </div>
          ) : (
            <article key={it.message.id} className="grid grid-cols-[36px_1fr] gap-2.5 px-4 py-1.5 hover:bg-surface-sunken">
              {(() => {
                const a = actors.data.get(it.message.author_id);
                return <Avatar kind={a?.kind ?? "human"} name={a ? (a.kind === "agent" ? a.handle : a.display_name) : "?"} emoji={a?.avatar_emoji} size="lg" />;
              })()}
              <div className="min-w-0">
                <div className="flex flex-wrap items-baseline gap-2">
                  <span className="font-semibold">{actors.data.get(it.message.author_id)?.display_name ?? "?"}</span>
                  <time className="text-meta text-fg-subtle" dateTime={it.message.created_at} title={fullTime(it.message.created_at, locale)}>
                    {messageTime(it.message.created_at, new Date(), locale)}
                  </time>
                </div>
                <p className="break-words whitespace-pre-wrap">{it.message.body}</p>
              </div>
            </article>
          ),
        )}
        <div ref={bottom} />
      </div>
      <form onSubmit={submit} className="m-4 mt-0 flex items-end gap-2 rounded-lg border border-border-strong bg-surface p-2 focus-within:border-fg-muted">
        <textarea
          aria-label={t("composer.label")}
          placeholder={t("composer.placeholder", { name: conversationTitle(channel, actors.data, meId) })}
          className="max-h-48 min-h-10 flex-1 resize-none bg-transparent px-2 py-1.5 outline-none placeholder:text-fg-subtle"
          rows={1}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              submit(e);
            }
          }}
        />
        <IconButton type="submit" label={t("composer.send")} icon={<SendHorizontal className="size-4" />} className="bg-accent text-accent-fg hover:bg-accent-hover hover:text-accent-fg" />
      </form>
    </div>
  );
}
