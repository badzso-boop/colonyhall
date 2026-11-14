import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { feedItems, useShortcut } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { ArrowUp, X } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { Skeleton } from "@/shared/ui";
import { useMarkRead, useMessages } from "../api";
import { mainStream, type Actor, type Channel } from "../model";
import { CompactionLive } from "@/features/compaction";
import { MessageItem, type MessageHandlers, type Viewer } from "./MessageItem";

const NEAR_BOTTOM = 80;
/** How many older pages a link may load while looking for its message. */
const MAX_JUMP_PAGES = 40;

/**
 * Scrolls a message into the middle of its scroller and marks it for a
 * moment (data-highlight, styled by MessageItem). Returns the clean-up.
 */
export function markMessage(root: HTMLElement | null, id: string): () => void {
  let el: HTMLElement | null = null;
  const frame = requestAnimationFrame(() => {
    el = root?.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(id)}"]`) ?? null;
    el?.scrollIntoView({ block: "center" });
    el?.setAttribute("data-highlight", "");
  });
  const timer = window.setTimeout(() => el?.removeAttribute("data-highlight"), 2500);
  return () => {
    cancelAnimationFrame(frame);
    window.clearTimeout(timer);
    el?.removeAttribute("data-highlight");
  };
}

/**
 * A conversation's feed (round 31, part 5): day separators, the
 * "Új üzenetek" divider at the first unread message, older pages when
 * scrolling up (keeping the position), a "↓ N új üzenet" pill while
 * scrolled up, and read marking while the end is in view.
 */
export function Feed({ channel, actors, viewer, handlers }: { channel: Channel; actors: Map<string, Actor>; viewer: Viewer; handlers: MessageHandlers }) {
  const lockedViewer = useMemo(() => ({ ...viewer, agentsLocked: Boolean(channel.agent_locked_at) }), [viewer, channel.agent_locked_at]);
  const { t, i18n } = useTranslation("conversation");
  const locale = localeOf(i18n.language);
  const messages = useMessages(channel.id);
  const markRead = useMarkRead();
  const scroller = useRef<HTMLDivElement>(null);
  const top = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const prevHeight = useRef<number | null>(null);
  const seenCount = useRef(0);
  const [unseen, setUnseen] = useState(0);
  // Round 55, part I: how many messages are below the view while scrolled
  // up, and whether the "Új üzenetek" divider is above the view.
  const [below, setBelow] = useState(0);
  const [dividerAbove, setDividerAbove] = useState(false);
  const [unreadDismissed, setUnreadDismissed] = useState(false);
  const divider = useRef<HTMLDivElement>(null);
  const [panes] = usePanes();
  // The first unread message when the conversation was opened (fixed while it is open).
  const [unreadFrom] = useState(() => channel.unread);

  const stream = useMemo(() => mainStream(messages.data ?? [], channel.kind), [messages.data, channel.kind]);
  const handles = useMemo(() => new Set([...actors.values()].map((a) => a.handle)), [actors]);
  const items = useMemo(
    () => feedItems(stream, new Date(), locale, { today: t("common:today"), yesterday: t("common:yesterday") }),
    [stream, locale, t],
  );
  const dividerAt = unreadFrom > 0 && unreadFrom < stream.length ? stream[stream.length - unreadFrom]?.id : undefined;

  // Where the view is: the end, messages below it, the divider above it.
  const measure = useCallback(() => {
    const el = scroller.current;
    if (!el) return;
    atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM;
    const bottom = el.scrollTop + el.clientHeight;
    let n = 0;
    if (!atBottom.current) el.querySelectorAll<HTMLElement>("[data-message-id]").forEach((m) => m.offsetTop > bottom && n++);
    setBelow(n);
    setDividerAbove(divider.current !== null && divider.current.offsetTop < el.scrollTop);
  }, []);

  const jumpToFirstUnread = useCallback(() => {
    divider.current?.scrollIntoView({ block: "start", behavior: "smooth" });
  }, []);

  const markIfVisible = useCallback(() => {
    // An admin observing a conversation they are not in has no read position (the server answers 404).
    if (atBottom.current && document.visibilityState === "visible" && channel.is_member && channel.unread > 0 && !markRead.isPending) markRead.mutate(channel.id);
  }, [channel.id, channel.is_member, channel.unread, markRead]);

  // Older pages: keep the visible message where it was.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (!el) return;
    if (prevHeight.current !== null) {
      el.scrollTop += el.scrollHeight - prevHeight.current;
      prevHeight.current = null;
      return;
    }
    const added = stream.length - seenCount.current;
    if (seenCount.current === 0 || atBottom.current) {
      el.scrollTop = el.scrollHeight;
      setUnseen(0);
    } else if (added > 0) {
      setUnseen((n) => n + added);
    }
    seenCount.current = stream.length;
  }, [stream.length]);

  // A link to a message (?msg=…, round 55): load older pages until it is
  // there, scroll it to the middle and mark it for a moment. A thread reply
  // (?thread=…&msg=…) is shown in the thread; here its root is the target.
  const [params] = useSearchParams();
  const jumpMsg = params.get("msg");
  const jumpThread = params.get("thread");
  const jumpDone = useRef<string | null>(null);
  const jumpTries = useRef(0);
  useEffect(() => {
    const key = jumpMsg ? `${jumpThread ?? ""}:${jumpMsg}` : null;
    if (!key || !jumpMsg || jumpDone.current === key || messages.isPending) return;
    const target = stream.some((m) => m.id === jumpMsg) ? jumpMsg : jumpThread && stream.some((m) => m.id === jumpThread) ? jumpThread : null;
    if (target) {
      jumpDone.current = key;
      jumpTries.current = 0;
      atBottom.current = false;
      seenCount.current = stream.length;
      return markMessage(scroller.current, target);
    }
    if (messages.hasNextPage && !messages.isFetchingNextPage && jumpTries.current < MAX_JUMP_PAGES) {
      jumpTries.current++;
      prevHeight.current = scroller.current?.scrollHeight ?? null;
      seenCount.current = Number.MAX_SAFE_INTEGER;
      void messages.fetchNextPage();
    } else if (!messages.hasNextPage || jumpTries.current >= MAX_JUMP_PAGES) {
      jumpDone.current = key; // not here (retracted, or too old): stay at the end
    }
  }, [jumpMsg, jumpThread, stream, messages]);

  useEffect(markIfVisible, [markIfVisible, stream.length]);
  // After the first layout (opened at the end): is the first unread above?
  useEffect(() => {
    const id = requestAnimationFrame(measure);
    return () => cancelAnimationFrame(id);
  }, [measure, stream.length]);

  // Esc marks the conversation read (unless a pane is open: Esc closes it);
  // Shift+Esc jumps to the first unread message.
  const panesOpen = panes.thread !== null || panes.tabs.length > 0;
  const markNow = () => {
    setUnreadDismissed(true);
    if (channel.is_member && (channel.unread > 0 || channel.mentions > 0)) markRead.mutate(channel.id);
  };
  useShortcut("escape", "conversation", "markRead", markNow, !panesOpen);
  useShortcut("shift+escape", "conversation", "firstUnread", jumpToFirstUnread, Boolean(dividerAt));
  useEffect(() => {
    document.addEventListener("visibilitychange", markIfVisible);
    return () => document.removeEventListener("visibilitychange", markIfVisible);
  }, [markIfVisible]);

  useEffect(() => {
    const el = top.current;
    if (!el || !messages.hasNextPage) return;
    const io = new IntersectionObserver((entries) => {
      if (entries[0]?.isIntersecting && !messages.isFetchingNextPage) {
        prevHeight.current = scroller.current?.scrollHeight ?? null;
        seenCount.current = Number.MAX_SAFE_INTEGER; // older messages are not "new"
        void messages.fetchNextPage().then(() => {
          seenCount.current = stream.length;
        });
      }
    });
    io.observe(el);
    return () => io.disconnect();
  }, [messages, stream.length]);

  if (messages.isPending) {
    return (
      <div className="flex flex-1 flex-col gap-3 p-4">
        <Skeleton className="h-4 w-1/3" />
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-4 w-1/2" />
      </div>
    );
  }

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={scroller}
        className="h-full overflow-y-auto py-2"
        onScroll={() => {
          measure();
          if (atBottom.current) {
            setUnseen(0);
            markIfVisible();
          }
        }}
      >
        <div ref={top} />
        {messages.isFetchingNextPage ? <p className="py-2 text-center text-meta text-fg-subtle">{t("message.loadingOlder")}</p> : null}
        {items.map((it) =>
          it.separator ? (
            <div
              key={`d-${it.day}`}
              className="my-3 flex items-center gap-3 px-4 text-meta font-medium text-fg-subtle before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border"
            >
              {it.label}
            </div>
          ) : (
            <div key={it.message.id}>
              {it.message.id === dividerAt ? (
                <div ref={divider} className="my-1 flex scroll-mt-12 items-center gap-3 px-4 text-meta font-semibold text-accent-text after:h-px after:flex-1 after:bg-accent">
                  {t("message.newMessages")}
                </div>
              ) : null}
              <MessageItem
                message={it.message}
                actors={actors}
                handles={handles}
                viewer={lockedViewer}
                locale={locale}
                summary={channel.kind !== "dm"}
                {...handlers}
              />
            </div>
          ),
        )}
        <CompactionLive channelId={channel.id} human={viewer.human} />
      </div>
      {dividerAt && dividerAbove && !unreadDismissed ? (
        <div className="absolute top-2 left-1/2 flex -translate-x-1/2 items-center overflow-hidden rounded-full bg-accent text-meta font-semibold text-accent-fg shadow-overlay">
          <button type="button" onClick={jumpToFirstUnread} className="flex items-center gap-1 py-1 pr-2 pl-3 hover:bg-accent-hover">
            <ArrowUp aria-hidden className="size-3.5" />
            {t("unread.first", { count: unreadFrom })}
          </button>
          <button type="button" aria-label={t("unread.markRead")} title={t("unread.markRead")} onClick={markNow} className="border-l border-accent-fg/30 py-1 pr-2.5 pl-2 hover:bg-accent-hover">
            <X aria-hidden className="size-3.5" />
          </button>
        </div>
      ) : null}
      {unseen > 0 || below > 0 ? (
        <button
          type="button"
          onClick={() => {
            const el = scroller.current;
            if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
          }}
          className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full bg-accent px-3 py-1 text-meta font-semibold text-accent-fg shadow-overlay"
        >
          {unseen > 0 ? t("message.jumpNew", { count: unseen }) : t("unread.latest", { count: below })}
        </button>
      ) : null}
    </div>
  );
}

