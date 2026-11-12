import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { CheckCheck, MessagesSquare } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { cn, hueVar } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Avatar, Badge, Button, EmptyState, PageHeader, Skeleton, toast } from "@/shared/ui";
import { useMarkAllThreadsRead, useMyThreads, type MyThread } from "../api";
import { useAttachments } from "../attachments";
import { Composer } from "../components/Composer";
import { MessageItem, type Viewer } from "../components/MessageItem";
import type { Actor } from "../model";
import { useViewer } from "../viewer";

function ReplyBox({ thread }: { thread: MyThread }) {
  const { t } = useTranslation("conversation");
  const tray = useAttachments(thread.channel_id);
  const memberIds = useMemo(() => new Set<string>(), []);
  return <Composer channelId={thread.channel_id} threadRootId={thread.root.id} placeholder={t("composer.threadPlaceholder")} memberIds={memberIds} tray={tray} />;
}

function ThreadCard({ thread, actors, viewer, onOpen }: { thread: MyThread; actors: Map<string, Actor>; viewer: Viewer; onOpen: () => void }) {
  const { t, i18n } = useTranslation("conversation");
  const locale = localeOf(i18n.language);
  const [replying, setReplying] = useState(false);
  const handles = useMemo(() => new Set([...actors.values()].map((a) => a.handle)), [actors]);
  const participants = (thread.root.reply_participant_ids ?? []).map((id) => actors.get(id)).filter((a): a is Actor => a !== undefined);
  return (
    <article aria-label={t("threads.card", { channel: thread.channel_name })} className="rounded-lg border border-border bg-surface">
      <header className="flex items-center gap-2 border-b border-border px-4 py-2 text-meta">
        <Link to={`/c/${thread.channel_id}`} className="font-semibold text-fg-muted hover:underline">
          {thread.channel_kind === "dm" ? t("threads.dm") : `# ${thread.channel_name}`}
        </Link>
        {thread.unread_replies > 0 ? (
          <Badge tone="danger">{t("threads.unread", { count: thread.unread_replies })}</Badge>
        ) : null}
        <span className="flex flex-1 justify-end">
          {participants.map((a, i) => (
            <Avatar key={a.id} kind={a.kind === "agent" ? "agent" : "human"} name={a.display_name || a.handle} emoji={a.avatar_emoji} size="sm" ringColor={hueVar(a.effective_color)} className={cn(i > 0 && "-ml-1.5")} />
          ))}
        </span>
      </header>
      <div className="py-1">
        <MessageItem message={thread.root} actors={actors} handles={handles} viewer={viewer} locale={locale} summary={false} />
        {thread.root.reply_count > thread.latest_replies.length ? (
          <button type="button" onClick={onOpen} className="px-4 text-meta text-accent-text hover:underline">
            {t("threads.more", { count: thread.root.reply_count - thread.latest_replies.length })}
          </button>
        ) : null}
        {thread.latest_replies.map((m) => (
          <MessageItem key={m.id} message={m} actors={actors} handles={handles} viewer={viewer} locale={locale} summary={false} />
        ))}
      </div>
      {replying ? <ReplyBox thread={thread} /> : null}
      <footer className="flex gap-2 border-t border-border px-4 py-2">
        {!replying ? (
          <Button size="sm" variant="ghost" onClick={() => setReplying(true)}>
            {t("threads.reply")}
          </Button>
        ) : null}
        <Button size="sm" variant="ghost" onClick={onOpen}>
          {t("threads.open")}
        </Button>
      </footer>
    </article>
  );
}

/** "Szálaim" (round 34, 2b): the threads the viewer wrote, replied in or was mentioned in. */
export default function ThreadsPage() {
  const { t } = useTranslation("conversation");
  const [unreadOnly, setUnreadOnly] = useState(false);
  const threads = useMyThreads(unreadOnly);
  const markAll = useMarkAllThreadsRead();
  const actors = useActors();
  const session = useSession();
  // The inline composer writes here, so the viewer can write in these threads.
  const viewer = useViewer(true);
  const [, dispatch] = usePanes();
  const list = threads.data?.pages.flatMap((p) => p.threads) ?? [];
  const unread = threads.data?.pages[0]?.unread_threads ?? 0;
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title={t("threads.title")}
        actions={
          <Button size="sm" variant="ghost" disabled={unread === 0} loading={markAll.isPending} onClick={() => markAll.mutate(undefined, { onError: (e) => toast(e.message, { tone: "danger" }) })}>
            <CheckCheck aria-hidden className="size-4" />
            {t("threads.markAll")}
          </Button>
        }
      />
      <div role="radiogroup" aria-label={t("threads.filter")} className="flex gap-1 px-4 pt-3">
        {[false, true].map((u) => (
          <button
            key={String(u)}
            type="button"
            role="radio"
            aria-checked={unreadOnly === u}
            onClick={() => setUnreadOnly(u)}
            className={cn("rounded-full border px-3 py-1 text-meta", unreadOnly === u ? "border-accent bg-accent-soft font-semibold" : "border-border text-fg-muted")}
          >
            {t(u ? "threads.unreadOnly" : "threads.all")}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {!threads.data || !actors.data || !session.data ? (
          <Skeleton className="h-24 w-full" />
        ) : list.length === 0 ? (
          <EmptyState icon={<MessagesSquare className="size-6" />} title={t(unreadOnly ? "threads.noneUnread" : "threads.none")} body={t("threads.noneBody")} />
        ) : (
          <div className="mx-auto flex max-w-3xl flex-col gap-3">
            {list.map((th) => (
              <ThreadCard key={th.root.id} thread={th} actors={actors.data} viewer={viewer} onOpen={() => dispatch({ type: "openThread", rootId: th.root.id })} />
            ))}
            {threads.hasNextPage ? (
              <Button variant="ghost" loading={threads.isFetchingNextPage} onClick={() => void threads.fetchNextPage()}>
                {t("threads.loadMore")}
              </Button>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}
