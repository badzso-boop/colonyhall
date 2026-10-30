import { memo, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Download, FileText, MessageSquare } from "lucide-react";
import { cn, fullTime, messageTime, relativeTime, type FileLink } from "@/shared/lib";
import { Avatar, Lightbox, Markdown } from "@/shared/ui";
import type { Actor } from "../model";
import type { Message } from "../api";

const LONG_CHARS = 1500;
const LONG_LINES = 20;

export type MessageHandlers = {
  onOpenThread?: (rootId: string) => void;
  onMention?: (handle: string) => void;
  onFileClick?: (author: Actor | undefined, link: FileLink) => void;
};

type Props = MessageHandlers & {
  message: Message;
  actors: Map<string, Actor>;
  handles: Set<string>;
  meId: string;
  locale: string;
  /** Show the "N válasz" row (the channel's main stream). */
  summary: boolean;
};

/** One message (docs/handoff/31, part 5). */
export const MessageItem = memo(function MessageItem({ message: m, actors, handles, meId, locale, summary, onOpenThread, onMention, onFileClick }: Props) {
  const { t } = useTranslation("conversation");
  const [expanded, setExpanded] = useState(false);
  const [lightbox, setLightbox] = useState<number | null>(null);
  const author = actors.get(m.author_id);
  const now = new Date();
  const mentionsMe = m.mentions.includes(meId);
  const long = m.body.length > LONG_CHARS || m.body.split("\n").length > LONG_LINES;
  const images = useMemo(
    () => (m.attachments ?? []).filter((a) => a.mime.startsWith("image/")).map((a) => ({ src: `/api/v1/files/${a.id}/content`, name: a.name })),
    [m.attachments],
  );
  const others = (m.attachments ?? []).filter((a) => !a.mime.startsWith("image/"));
  const retracted = Boolean(m.deleted_at);
  const name = author ? (author.kind === "agent" ? author.display_name || author.handle : author.display_name || author.handle) : "?";

  return (
    <article
      data-message-id={m.id}
      tabIndex={-1}
      onKeyDown={(e) => {
        // "r": reply in thread on the focused message (listed in the shortcut help).
        if (e.key === "r" && !e.ctrlKey && !e.metaKey && !e.altKey && summary && onOpenThread && e.target === e.currentTarget) {
          e.preventDefault();
          onOpenThread(m.id);
        }
      }}
      className={cn(
        "group grid grid-cols-[36px_1fr] gap-x-2.5 px-4 py-1.5 outline-none hover:bg-surface-sunken focus-visible:bg-surface-sunken focus-visible:shadow-[inset_2px_0_0_var(--color-focus-ring)]",
        mentionsMe && "bg-accent-soft shadow-[inset_2px_0_0_var(--color-accent)] hover:bg-accent-soft",
      )}
    >
      <Avatar kind={author?.kind ?? "human"} name={author?.handle ?? "?"} emoji={author?.avatar_emoji} size="lg" />
      <div className="min-w-0">
        <header className="flex flex-wrap items-baseline gap-x-2">
          <span className="font-semibold">{name}</span>
          {author?.kind === "agent" ? <span className="font-mono text-meta text-fg-muted">@{author.handle}</span> : null}
          <time className="text-meta text-fg-subtle" dateTime={m.created_at} title={fullTime(m.created_at, locale)}>
            {messageTime(m.created_at, now, locale)}
          </time>
        </header>
        {retracted ? (
          <p className="text-fg-subtle italic">🗑 {t("message.retracted")}</p>
        ) : (
          <div className={cn("relative", long && !expanded && "max-h-80 overflow-hidden")}>
            <Markdown
              text={m.body}
              inChat
              knownHandles={handles}
              onMention={onMention}
              onFileClick={onFileClick ? (link) => onFileClick(author, link) : undefined}
              canOpenFile={() => author?.kind === "agent"}
              copyLabel={t("message.copy")}
              copiedLabel={t("message.copied")}
            />
            {long && !expanded ? <span aria-hidden className="absolute inset-x-0 bottom-0 h-12 bg-linear-to-t from-bg to-transparent" /> : null}
          </div>
        )}
        {long && !retracted ? (
          <button type="button" onClick={() => setExpanded(!expanded)} className="text-meta font-medium text-accent-text hover:underline">
            {expanded ? t("message.less") : t("message.more")}
          </button>
        ) : null}
        {images.length ? (
          <div className="mt-1.5 flex flex-wrap gap-2">
            {images.map((img, i) => (
              <button key={img.src} type="button" onClick={() => setLightbox(i)} className="overflow-hidden rounded-md border border-border">
                <img src={img.src} alt={img.name} loading="lazy" className="max-h-44 max-w-60 object-cover" />
              </button>
            ))}
          </div>
        ) : null}
        {others.length ? (
          <div className="mt-1.5 flex flex-wrap gap-2">
            {others.map((a) => (
              <a
                key={a.id}
                href={`/api/v1/files/${a.id}/content`}
                download={a.name}
                className="inline-flex items-center gap-1.5 rounded-md border border-border bg-surface px-2 py-1 text-meta hover:border-border-strong"
              >
                <FileText aria-hidden className="size-3.5 text-fg-subtle" />
                <span className="max-w-48 truncate">{a.name}</span>
                <span className="tabular text-fg-subtle">{Math.max(1, Math.round(a.size / 1024))} KB</span>
                <Download aria-hidden className="size-3.5 text-fg-subtle" />
              </a>
            ))}
          </div>
        ) : null}
        {m.reactions?.length ? (
          <div className="mt-1 flex flex-wrap gap-1">
            {m.reactions.map((r) => (
              <span key={r.emoji} className={cn("tabular inline-flex h-6 items-center gap-1 rounded-full border px-2 text-meta", r.actor_ids.includes(meId) ? "border-accent bg-accent-soft" : "border-border bg-surface")}>
                {r.emoji} {r.count}
              </span>
            ))}
          </div>
        ) : null}
        {summary && m.reply_count > 0 && onOpenThread ? (
          <button
            type="button"
            onClick={() => onOpenThread(m.id)}
            className="mt-1 inline-flex items-center gap-2 rounded-md border border-transparent py-1 pr-2 pl-1 text-meta text-fg-muted hover:border-border hover:bg-surface"
          >
            <span className="flex">
              {(m.reply_participant_ids ?? []).map((id, i) => {
                const a = actors.get(id);
                return <Avatar key={id} kind={a?.kind ?? "human"} name={a?.handle ?? "?"} emoji={a?.avatar_emoji} size="sm" className={i ? "-ml-1" : ""} />;
              })}
            </span>
            <span className="font-semibold text-accent-text">{t("message.replies", { count: m.reply_count })}</span>
            {m.last_reply_at ? <span>· {t("message.lastReply", { when: relativeTime(m.last_reply_at, now, locale) })}</span> : null}
          </button>
        ) : null}
        {!summary || m.reply_count > 0 || !onOpenThread ? null : (
          <button
            type="button"
            onClick={() => onOpenThread(m.id)}
            className="mt-0.5 hidden items-center gap-1 text-meta text-fg-subtle group-hover:inline-flex hover:text-fg"
          >
            <MessageSquare aria-hidden className="size-3.5" />
            {t("message.replyInThread")}
          </button>
        )}
      </div>
      {lightbox !== null ? (
        <Lightbox
          images={images}
          start={lightbox}
          onClose={() => setLightbox(null)}
          labels={{ close: t("common:close"), prev: t("message.prev"), next: t("message.next"), download: t("message.download") }}
        />
      ) : null}
    </article>
  );
});
