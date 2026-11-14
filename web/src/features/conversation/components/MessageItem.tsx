import { memo, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { AlarmClock, ArrowUpRight, Bookmark, BookmarkCheck, Forward, Link2, CornerDownRight, CornerUpLeft, Download, FileText, Flag, MessageSquare, MoreHorizontal, Pin, PinOff, SquareTerminal, Undo2 } from "lucide-react";
import { chipRefs, cn, fullTime, hueVar, messageChipRef, messageHref, messageTime, messageURL, relativeTime, type Chip, type FileLink } from "@/shared/lib";
import { Avatar, Badge, Button, IconButton, Lightbox, Markdown, Menu, MenuContent, MenuItem, MenuTrigger, Tooltip, toast } from "@/shared/ui";
import { EmojiButton } from "@/features/emoji";
import { ErrorChip, StepDrawer } from "@/features/agents";
import { ApprovalCard, DelegationActions } from "@/features/approvals";
import { CompactionCard } from "@/features/compaction";
import { DecisionChip } from "@/features/decisions";
import { MessageFileSheet } from "@/features/files";
import { QuestionChip } from "@/features/questions";
import { useSaveActions } from "@/features/saved";
import type { Actor } from "../model";
import { useRetract, useSetPin, useToggleReaction, type Message } from "../api";
import { isRateLimitNote, REACTIONS, terminalNotice } from "../messageParts";
import { ForwardDialog } from "./ForwardDialog";
import { LinkCards } from "./LinkCards";
import { ProfileTrigger } from "./ProfilePopover";

const LONG_CHARS = 1500;
const LONG_LINES = 20;

export type MessageHandlers = {
  onOpenThread?: (rootId: string) => void;
  onMention?: (handle: string) => void;
  /** A relative file reference (path:12-20): opens the agent's file browser. */
  onFileClick?: (author: Actor | undefined, link: FileLink) => void;
  /** "Terminál megtekintése" under a terminal notice: opens that terminal in the work pane. */
  onOpenTerminal?: (agentId: string, name: string) => void;
};

/** Who is looking: humans see details (errors, approvals, steps, files); agents never do. */
export type Viewer = { agentsLocked?: boolean; id: string; human: boolean; admin: boolean; canWrite: boolean; pausedAgents: Set<string> };

type Props = MessageHandlers & {
  message: Message;
  actors: Map<string, Actor>;
  handles: Set<string>;
  viewer: Viewer;
  locale: string;
  /** Show the "N válasz" row (the channel's main stream). */
  summary: boolean;
};

/** "Visszavonás" from the message menu, confirmed in place (round 26). */
function RetractConfirm({ messageId, onDone }: { messageId: string; onDone: () => void }) {
  const { t } = useTranslation("conversation");
  const retract = useRetract();
  return (
    <div role="group" aria-label={t("message.retractConfirm")} className="mt-1 flex flex-wrap items-center gap-2 rounded-md border border-danger/40 bg-danger-soft px-2.5 py-1.5 text-ui">
      <span className="flex-1">{t("message.retractConfirm")}</span>
      <Button
        size="sm"
        variant="danger"
        loading={retract.isPending}
        onClick={() =>
          retract.mutate(messageId, {
            onSuccess: onDone,
            onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }),
          })
        }
      >
        {t("message.retract")}
      </Button>
      <Button size="sm" variant="ghost" onClick={onDone}>
        {t("message.cancel")}
      </Button>
    </div>
  );
}

/** One message (round 31, part 5; the elements of round 34, part 6). */
export const MessageItem = memo(function MessageItem({ message: m, actors, handles, viewer, locale, summary, onOpenThread, onMention, onFileClick, onOpenTerminal }: Props) {
  const { t } = useTranslation("conversation");
  const [expanded, setExpanded] = useState(false);
  const [lightbox, setLightbox] = useState<number | null>(null);
  const [confirmRetract, setConfirmRetract] = useState(false);
  const [file, setFile] = useState<FileLink | null>(null);
  const [forwarding, setForwarding] = useState(false);
  const react = useToggleReaction();
  const setPin = useSetPin();
  const saveActions = useSaveActions(m.id, viewer.human && !m.deleted_at);
  const author = actors.get(m.author_id);
  const fromAgent = author?.kind === "agent";
  const now = new Date();
  const mentionsMe = m.mentions.includes(viewer.id);
  const long = m.body.length > LONG_CHARS || m.body.split("\n").length > LONG_LINES;
  const images = useMemo(
    () => (m.attachments ?? []).filter((a) => a.mime.startsWith("image/")).map((a) => ({ src: `/api/v1/files/${a.id}/content`, name: a.name })),
    [m.attachments],
  );
  const others = (m.attachments ?? []).filter((a) => !a.mime.startsWith("image/"));
  const retracted = Boolean(m.deleted_at);
  const name = author ? author.display_name || author.handle : "?";
  const paused = fromAgent && (author.state === "paused" || viewer.pausedAgents.has(author.id));
  const approvals = viewer.human && !retracted ? chipRefs(m.body, "approval") : [];
  const terminal = viewer.human && fromAgent && !retracted ? terminalNotice(m.body) : null;
  const canRetract = viewer.human && m.author_id === viewer.id && !retracted;
  const canPin = viewer.human && viewer.canWrite && !retracted;
  const copyLink = () =>
    void navigator.clipboard
      .writeText(messageURL(m, window.location.origin))
      .then(() => toast(t("actions.linkCopied")))
      .catch(() => toast(t("actions.linkFailed"), { tone: "danger" }));
  const togglePin = () =>
    setPin.mutate(
      { messageId: m.id, pinned: !m.pinned },
      {
        onSuccess: () => toast(t(m.pinned ? "pins.unpinned" : "pins.pinned")),
        onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }),
      },
    );

  // The rate-limit note (round 27): a muted system line, not a message.
  if (!retracted && isRateLimitNote(m.body, author?.kind)) {
    return (
      <p data-message-id={m.id} role="note" className="flex items-baseline gap-2 px-4 py-0.5 pl-[3.6rem] text-meta text-fg-muted">
        <span className="font-mono">@{author?.handle}</span>
        <span className="min-w-0 flex-1">{m.body}</span>
        <time className="text-fg-subtle" dateTime={m.created_at} title={fullTime(m.created_at, locale)}>
          {messageTime(m.created_at, now, locale)}
        </time>
      </p>
    );
  }

  const renderChip = (c: Chip): ReactNode => {
    switch (c.kind) {
      case "question":
        return <QuestionChip number={c.ref} />;
      case "decision":
        return viewer.human ? <DecisionChip number={c.ref} /> : c.raw;
      case "error":
        return <ErrorChip errorId={c.ref} human={viewer.human} />;
      case "approval":
        // Shown as a card under the message (humans); never as text.
        return null;
      case "message":
        // A link to another message (a reminder, a forward): it opens there,
        // or the conversation's "not found" for someone without access.
        return (
          <Link to={messageHref(messageChipRef(c.ref))} className="inline-flex items-center gap-0.5 font-medium text-accent-text hover:underline">
            {t("message.openLinked")}
            <ArrowUpRight aria-hidden className="size-3.5" />
          </Link>
        );
    }
  };

  const openFile = (link: FileLink) => {
    if (link.isRef) onFileClick?.(author, link);
    else setFile(link);
  };

  return (
    <article
      data-message-id={m.id}
      tabIndex={-1}
      onKeyDown={(e) => {
        // Keys on the focused message (listed in the shortcut help): r reply
        // in thread, s save, l copy link, f forward, p pin.
        if (e.ctrlKey || e.metaKey || e.altKey || e.target !== e.currentTarget || retracted) return;
        const act: Record<string, (() => void) | undefined> = {
          r: summary && onOpenThread ? () => onOpenThread(m.thread_root_id ?? m.id) : undefined,
          s: viewer.human ? saveActions.toggle : undefined,
          l: viewer.human ? copyLink : undefined,
          f: viewer.human ? () => setForwarding(true) : undefined,
          p: canPin ? togglePin : undefined,
        };
        const run = act[e.key];
        if (run) {
          e.preventDefault();
          run();
        }
      }}
      className={cn(
        "group relative grid grid-cols-[36px_1fr] gap-x-2.5 px-4 py-1.5 outline-none hover:bg-surface-sunken focus-visible:bg-surface-sunken focus-visible:shadow-[inset_2px_0_0_var(--color-focus-ring)]",
        mentionsMe && "bg-accent-soft shadow-[inset_2px_0_0_var(--color-accent)] hover:bg-accent-soft",
        m.important && !retracted && "shadow-[inset_2px_0_0_var(--color-warning)]",
        // A link pointed here (markMessage sets it for a moment).
        "data-highlight:bg-warning-soft motion-safe:transition-colors",
      )}
    >
      {author ? (
        <ProfileTrigger actorId={author.id} className="self-start">
          <Avatar kind={author.kind} name={author.handle} emoji={author.avatar_emoji} size="lg" status={paused ? "paused" : undefined} className={viewer.agentsLocked && author.kind === "agent" ? "opacity-50" : undefined} ringColor={author.kind === "agent" ? hueVar(author.effective_color) : undefined} />
        </ProfileTrigger>
      ) : (
        <Avatar kind="human" name="?" size="lg" />
      )}
      <div className="min-w-0">
        <header className="flex flex-wrap items-baseline gap-x-2">
          {author ? (
            <ProfileTrigger actorId={author.id} className="font-semibold hover:underline">
              {name}
            </ProfileTrigger>
          ) : (
            <span className="font-semibold">{name}</span>
          )}
          {fromAgent ? <span className="font-mono text-meta text-fg-muted">@{author.handle}</span> : null}
          {paused ? <Badge tone="warning">{t("message.paused")}</Badge> : null}
          <time className="text-meta text-fg-subtle" dateTime={m.created_at} title={fullTime(m.created_at, locale)}>
            {messageTime(m.created_at, now, locale)}
          </time>
          {m.important && !retracted ? (
            <Badge tone="warning">
              <Flag aria-hidden className="mr-1 inline size-3" />
              {t("pins.important")}
            </Badge>
          ) : null}
          {m.forwarded_from_id && !retracted ? <span className="text-meta text-fg-subtle">{t("actions.forwarded")}</span> : null}
          {m.pinned && !retracted ? (
            <Tooltip content={t("pins.pinnedHere")}>
              <Pin aria-label={t("pins.pinnedHere")} className="size-3.5 self-center text-fg-subtle" />
            </Tooltip>
          ) : null}
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
              onFileClick={openFile}
              // Local files of an agent open for humans only (round 16).
              canOpenFile={(link) => fromAgent && viewer.human && (!link.isRef || onFileClick !== undefined)}
              renderChip={renderChip}
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
        {approvals.map((id) => (
          <ApprovalCard key={id} id={id} />
        ))}
        {terminal ? (
          <Button size="sm" variant="secondary" className="mt-1" icon={<SquareTerminal className="size-3.5" />} onClick={() => onOpenTerminal?.(m.author_id, terminal)}>
            {t("message.viewTerminal")}
          </Button>
        ) : null}
        {m.compaction_id && !retracted ? <CompactionCard compactionId={m.compaction_id} attachments={m.attachments} canWrite={viewer.human && viewer.canWrite} /> : null}
        {images.length && !retracted && !m.compaction_id ? (
          <div className="mt-1.5 flex flex-wrap gap-2">
            {images.map((img, i) => (
              <button key={img.src} type="button" onClick={() => setLightbox(i)} className="overflow-hidden rounded-md border border-border">
                <img src={img.src} alt={img.name} loading="lazy" className="max-h-44 max-w-60 object-cover" />
              </button>
            ))}
          </div>
        ) : null}
        {others.length && !retracted && !m.compaction_id ? (
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
        {m.link_cards?.length && !retracted ? <LinkCards cards={m.link_cards} /> : null}
        {viewer.human && fromAgent && !retracted ? <DelegationActions message={m} admin={viewer.admin} /> : null}
        {summary && m.thread_root_id && m.also_in_channel && onOpenThread ? (
          <button type="button" onClick={() => onOpenThread(m.thread_root_id!)} className="mt-1 inline-flex items-center gap-1 text-meta text-accent-text hover:underline">
            <CornerDownRight aria-hidden className="size-3.5" />
            {t("message.repliedInThread")}
          </button>
        ) : null}
        {m.origin ? (
          <Link to={`/c/${m.origin.channel_id}?thread=${m.origin.thread_id}`} className="mt-1 inline-flex items-center gap-1 text-meta text-accent-text hover:underline">
            <CornerUpLeft aria-hidden className="size-3.5" />
            {t("message.fromThread", { channel: m.origin.channel })}
          </Link>
        ) : null}
        {m.answers ? (
          <Link to={`/c/${m.answers.channel_id}`} className="mt-1 inline-flex items-center gap-1 text-meta text-accent-text hover:underline">
            <CornerDownRight aria-hidden className="size-3.5" />
            {t("message.answersDm", { handle: actors.get(m.answers.author_id)?.handle ?? "?" })}
          </Link>
        ) : null}
        {viewer.human && fromAgent && !retracted ? <StepDrawer messageId={m.id} /> : null}
        {m.reactions?.length && !retracted ? (
          <div className="mt-1 flex flex-wrap gap-1">
            {m.reactions.map((r) => {
              const mine = r.actor_ids.includes(viewer.id);
              return (
                <button
                  key={r.emoji}
                  type="button"
                  aria-pressed={mine}
                  title={r.actor_ids.map((id) => `@${actors.get(id)?.handle ?? "?"}`).join(", ")}
                  onClick={() => react.mutate({ messageId: m.id, emoji: r.emoji, mine })}
                  className={cn("tabular inline-flex h-6 items-center gap-1 rounded-full border px-2 text-meta", mine ? "border-accent bg-accent-soft" : "border-border bg-surface hover:border-border-strong")}
                >
                  {r.emoji} {r.count}
                </button>
              );
            })}
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
        {confirmRetract ? <RetractConfirm messageId={m.id} onDone={() => setConfirmRetract(false)} /> : null}
      </div>
      {!retracted ? (
        <div
          role="toolbar"
          aria-label={t("message.actions")}
          className="absolute -top-3 right-4 hidden items-center gap-0.5 rounded-md border border-border bg-surface-raised p-0.5 shadow-raised group-focus-within:flex group-hover:flex has-[[data-state=open]]:flex"
        >
          {REACTIONS.map((e) => {
            const mine = m.reactions?.some((r) => r.emoji === e && r.actor_ids.includes(viewer.id)) ?? false;
            return (
              <Tooltip key={e} content={t(`message.reaction.${e === "👍" ? "agree" : "disagree"}`)}>
                <button
                  type="button"
                  aria-label={t(`message.reaction.${e === "👍" ? "agree" : "disagree"}`)}
                  aria-pressed={mine}
                  onClick={() => react.mutate({ messageId: m.id, emoji: e, mine })}
                  className={cn("grid size-7 place-items-center rounded-sm text-ui hover:bg-surface-sunken", mine && "bg-accent-soft")}
                >
                  {e}
                </button>
              </Tooltip>
            );
          })}
          <EmojiButton
            size="sm"
            label={t("message.reaction.add")}
            onPick={(e) => react.mutate({ messageId: m.id, emoji: e, mine: m.reactions?.some((r) => r.emoji === e && r.actor_ids.includes(viewer.id)) ?? false })}
          />
          {viewer.human ? (
            <Menu>
              <MenuTrigger asChild>
                <IconButton size="sm" label={t("message.menu")} icon={<MoreHorizontal className="size-4" />} />
              </MenuTrigger>
              <MenuContent>
                {onOpenThread ? (
                  <MenuItem icon={<MessageSquare aria-hidden className="size-4" />} onSelect={() => onOpenThread(m.thread_root_id ?? m.id)}>
                    {t("message.replyInThread")}
                  </MenuItem>
                ) : null}
                <MenuItem icon={<Link2 aria-hidden className="size-4" />} onSelect={copyLink}>
                  {t("actions.copyLink")}
                </MenuItem>
                <MenuItem icon={<Forward aria-hidden className="size-4" />} onSelect={() => setForwarding(true)}>
                  {t("actions.forward")}
                </MenuItem>
                <MenuItem icon={saveActions.saved ? <BookmarkCheck aria-hidden className="size-4" /> : <Bookmark aria-hidden className="size-4" />} onSelect={saveActions.toggle}>
                  {t(saveActions.saved ? "actions.unsave" : "actions.save")}
                </MenuItem>
                <MenuItem icon={<AlarmClock aria-hidden className="size-4" />} onSelect={saveActions.remind}>
                  {t("actions.remind")}
                </MenuItem>
                {canPin ? (
                  <MenuItem icon={m.pinned ? <PinOff aria-hidden className="size-4" /> : <Pin aria-hidden className="size-4" />} onSelect={togglePin}>
                    {t(m.pinned ? "pins.unpin" : "pins.pin")}
                  </MenuItem>
                ) : null}
                {canRetract ? (
                  <MenuItem danger icon={<Undo2 aria-hidden className="size-4" />} onSelect={() => setConfirmRetract(true)}>
                    {t("message.retract")}
                  </MenuItem>
                ) : null}
              </MenuContent>
            </Menu>
          ) : null}
        </div>
      ) : null}
      {saveActions.dialog}
      {forwarding ? <ForwardDialog message={m} onClose={() => setForwarding(false)} /> : null}
      {lightbox !== null ? (
        <Lightbox
          images={images}
          start={lightbox}
          onClose={() => setLightbox(null)}
          labels={{ close: t("common:close"), prev: t("message.prev"), next: t("message.next"), download: t("message.download") }}
        />
      ) : null}
      {file ? (
        <MessageFileSheet messageId={m.id} path={file.path} handle={author?.handle ?? ""} lines={{ start: file.startLine, end: file.endLine }} onClose={() => setFile(null)} />
      ) : null}
    </article>
  );
});
