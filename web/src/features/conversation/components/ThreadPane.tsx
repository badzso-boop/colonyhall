import { useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { usePanes } from "@/shared/panes";
import { EmptyState, IconButton, Skeleton } from "@/shared/ui";
import { useActors, useSession } from "@/features/identity";
import { useChannels, useMarkThreadRead, useThread } from "../api";
import { useAttachments } from "../attachments";
import { openAgent, openFiles } from "../workPanes";
import { ActivityLine } from "./ActivityLine";
import { Composer } from "./Composer";
import { DropZone } from "./DropZone";
import { MessageItem } from "./MessageItem";

/** The thread pane (docs/handoff/31, part 5): the root, its replies, a composer of its own. */
export function ThreadPane({ rootId }: { rootId: string }) {
  const { t, i18n } = useTranslation("conversation");
  const locale = localeOf(i18n.language);
  const [, dispatch] = usePanes();
  const thread = useThread(rootId);
  const actors = useActors();
  const channels = useChannels();
  const session = useSession();
  const meId = session.data?.actor.id ?? "";
  const channelId = thread.data?.root.channel_id ?? "";
  const tray = useAttachments(channelId);
  const channel = channels.data?.find((c) => c.id === channelId);
  const handles = useMemo(() => new Set([...(actors.data?.values() ?? [])].map((a) => a.handle)), [actors.data]);
  const memberIds = useMemo(() => new Set(channel?.member_ids ?? []), [channel]);
  // "Szálaim": seeing the thread to its end marks it read (debounced, like the channel read).
  const markRead = useMarkThreadRead();
  const human = session.data?.actor.kind === "human";
  const lastReply = thread.data?.replies.at(-1)?.id;
  const rootLoaded = thread.data?.root.id;
  useEffect(() => {
    if (!human || !rootLoaded || !lastReply) return;
    const id = window.setTimeout(() => markRead.mutate(rootLoaded), 1000);
    return () => window.clearTimeout(id);
    // markRead is a fresh object each render; the ids are what matter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [human, rootLoaded, lastReply]);

  const header = (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
      <h2 className="text-title font-semibold">{t("message.thread")}</h2>
      {channel && channel.kind !== "dm" ? <span className="truncate text-ui text-fg-muted"># {channel.name}</span> : null}
      <IconButton className="ml-auto" label={t("message.closeThread")} icon={<X className="size-4" />} onClick={() => dispatch({ type: "closeThread" })} />
    </header>
  );
  if (thread.isError) return <div className="flex h-full flex-col">{header}<EmptyState title={t("notFound")} /></div>;
  if (!thread.data || !actors.data) return <div className="flex h-full flex-col">{header}<Skeleton className="m-4 h-4 w-2/3" /></div>;

  const handlers = {
    onMention: (handle: string) => {
      const a = [...actors.data.values()].find((x) => x.handle === handle);
      if (a?.kind === "agent") dispatch(openAgent(a.id));
    },
    onFileClick: (author: { id: string; kind: string } | undefined, link: Parameters<typeof openFiles>[1]) => {
      if (author?.kind === "agent") dispatch(openFiles(author.id, link));
    },
  };

  return (
    <DropZone onFiles={tray.add} className="flex h-full min-h-0 flex-col">
      {header}
      <div className="min-h-0 flex-1 overflow-y-auto py-2">
        <MessageItem message={thread.data.root} actors={actors.data} handles={handles} meId={meId} locale={locale} summary={false} {...handlers} />
        <div className="my-2 flex items-center gap-3 px-4 text-meta text-fg-subtle after:h-px after:flex-1 after:bg-border">
          {t("message.replies", { count: thread.data.replies.length })}
        </div>
        {thread.data.replies.map((m) => (
          <MessageItem key={m.id} message={m} actors={actors.data} handles={handles} meId={meId} locale={locale} summary={false} {...handlers} />
        ))}
      </div>
      <ActivityLine channelId={channelId} threadRootId={rootId} actors={actors.data} />
      <Composer channelId={channelId} threadRootId={rootId} placeholder={t("composer.threadPlaceholder")} memberIds={memberIds} tray={tray} readOnly={Boolean(channel?.archived_at)} />
    </DropZone>
  );
}
