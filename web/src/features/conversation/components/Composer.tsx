import { useEffect, useMemo, useRef, useState, type ClipboardEvent, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { CalendarClock, Camera, FileText, FolderOpen, Image as ImageIcon, Paperclip, Plus, RotateCw, SendHorizontal, SquareTerminal, X } from "lucide-react";
import { activeMention, cn, commandList, insertMention, openScheduleBuilder, parseCommand, randomId, rankMentions, useComposerInsert, useShortcut } from "@/shared/lib";
import { useWidth } from "@/shared/hooks";
import { Avatar, IconButton, Menu, MenuContent, MenuItem, MenuTrigger, toast } from "@/shared/ui";
import { useActors, useSession } from "@/features/identity";
import { answerSuggestions, useAnswerQuestion, useOpenQuestions } from "@/features/questions";
import { useCreateSchedule, useRetract, useSendMessage } from "../api";
import type { AttachmentTray } from "../attachments";

type Props = {
  channelId: string;
  threadRootId?: string;
  placeholder: string;
  memberIds: Set<string>;
  tray: AttachmentTray;
  readOnly?: boolean;
  /** Opens the work pane for the conversation's agent; absent when there is none. */
  onOpenFiles?: () => void;
  onOpenTerminals?: () => void;
};

function Tray({ tray }: { tray: AttachmentTray }) {
  const { t } = useTranslation("conversation");
  if (!tray.items.length) return null;
  return (
    <ul className="flex flex-wrap gap-2 px-2.5 pt-2.5">
      {tray.items.map((a) => (
        <li key={a.key} className="relative flex min-w-48 items-center gap-2 overflow-hidden rounded-md border border-border bg-surface-sunken py-1.5 pr-1 pl-2 text-meta">
          <span className="grid size-7 place-items-center rounded-sm bg-surface text-fg-muted">
            {a.file.type.startsWith("image/") ? <ImageIcon aria-hidden className="size-4" /> : <FileText aria-hidden className="size-4" />}
          </span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate text-ui">{a.file.name}</span>
            <span className={cn("tabular", a.status === "error" ? "text-danger" : "text-fg-subtle")}>
              {a.status === "uploading"
                ? t("composer.uploading", { pct: Math.round(a.progress * 100) })
                : a.status === "error"
                  ? t(a.error === "too_large" ? "composer.tooLarge" : "composer.failed")
                  : `${Math.max(1, Math.round(a.file.size / 1024))} KB`}
            </span>
          </span>
          {a.status === "error" && a.error !== "too_large" ? (
            <IconButton size="sm" label={t("composer.retry")} icon={<RotateCw className="size-3.5" />} onClick={() => tray.retry(a)} />
          ) : null}
          <IconButton size="sm" label={t("composer.remove", { name: a.file.name })} icon={<X className="size-3.5" />} onClick={() => tray.remove(a.key)} />
          {a.status === "uploading" ? <span aria-hidden className="absolute bottom-0 left-0 h-0.5 bg-accent" style={{ width: `${Math.round(a.progress * 100)}%` }} /> : null}
        </li>
      ))}
    </ul>
  );
}

/** The message composer (round 31, part 5). */
export function Composer({ channelId, threadRootId, placeholder, memberIds, tray, readOnly, onOpenFiles, onOpenTerminals }: Props) {
  const { t } = useTranslation("conversation");
  const width = useWidth();
  const actors = useActors();
  const session = useSession();
  const send = useSendMessage(channelId);
  const retract = useRetract();
  const schedule = useCreateSchedule();
  const answer = useAnswerQuestion();
  const openQuestions = useOpenQuestions();
  const [text, setText] = useState("");
  const [caret, setCaret] = useState(0);
  const [pick, setPick] = useState(0);
  const area = useRef<HTMLTextAreaElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const cameraInput = useRef<HTMLInputElement>(null);

  // A line reference (or other text) sent here from elsewhere, e.g. the file browser.
  const insert = useComposerInsert();
  const target = threadRootId ? "thread" : "main";
  const seen = useRef(insert.nonce);
  useEffect(() => {
    if (insert.nonce === seen.current || insert.target !== target) return;
    seen.current = insert.nonce;
    setText((s) => (s && !s.endsWith(" ") ? `${s} ` : s) + insert.text + " ");
    area.current?.focus();
  }, [insert, target]);

  // Listed in the help; the composer handles them in onKeyDown.
  useShortcut("enter", "composer", "send", undefined, !threadRootId);
  useShortcut("shift+enter", "composer", "newline", undefined, !threadRootId);
  useShortcut("arrowup", "composer", "lastMessage", undefined, !threadRootId);

  const mention = activeMention(text, caret);
  const mentionList = useMemo(
    () => (mention && actors.data ? rankMentions(mention.query, [...actors.data.values()], memberIds) : []),
    [mention, actors.data, memberIds],
  );
  const slash = /^\/\w*$/.test(text) ? commandList.filter((c) => `/${c.name}`.startsWith(text)) : [];
  const questionList = answerSuggestions(text, openQuestions.data ?? []);
  const popup: "mention" | "slash" | "question" | null = mentionList.length ? "mention" : slash.length ? "slash" : questionList.length ? "question" : null;
  const options = popup === "mention" ? mentionList.length : popup === "question" ? questionList.length : slash.length;

  if (readOnly) {
    return <p className="m-4 mt-0 rounded-lg border border-border bg-surface-sunken px-3 py-2.5 text-ui text-fg-muted">{t("composer.readOnly")}</p>;
  }

  const choose = (i: number) => {
    if (popup === "mention") {
      const a = mentionList[i];
      if (!a) return;
      const next = insertMention(text, caret, a.handle);
      setText(next.text);
      setCaret(next.caret);
      requestAnimationFrame(() => area.current?.setSelectionRange(next.caret, next.caret));
    } else if (popup === "slash") {
      const c = slash[i];
      if (c) setText(`/${c.name} `);
    } else if (popup === "question") {
      const q = questionList[i];
      if (q) setText(`/answer ${q.formatted_number} `);
    }
    setPick(0);
    area.current?.focus();
  };

  const submit = async () => {
    const body = text.trim();
    if (tray.busy) {
      toast(t("composer.waiting"));
      return;
    }
    if (!body && !tray.fileIds.length) return;
    const cmd = parseCommand(body);
    try {
      if (cmd.kind === "invalid") {
        toast(t(cmd.name === "answer" ? "composer.answerHelp" : "composer.scheduleHelp"), { tone: "danger" });
        return;
      }
      if (cmd.kind === "schedule") {
        const s = await schedule.mutateAsync({ channel_id: channelId, thread_root_id: threadRootId, body: cmd.body, at: cmd.at, cron: cmd.cron });
        toast(t("composer.scheduled", { when: new Date(s.next_run_at).toLocaleString() }));
      } else if (cmd.kind === "answer") {
        const q = await answer.mutateAsync({ id: cmd.question, answer: cmd.answer });
        toast(t("composer.answered", { q: q.formatted_number }));
      } else {
        const res = await send.mutateAsync({
          body: body || tray.items.map((a) => a.file.name).join(", "),
          client_msg_id: randomId(),
          ...(threadRootId ? { thread_root_id: threadRootId } : {}),
          ...(tray.fileIds.length ? { file_ids: tray.fileIds } : {}),
        });
        tray.clear();
        const id = res.message?.id;
        if (id && session.data?.actor.kind === "human") {
          toast(t("composer.sent"), { action: { label: t("composer.undo"), run: () => retract.mutate(id) }, ms: 10_000 });
        }
      }
      setText("");
      setCaret(0);
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { tone: "danger" });
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (popup) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault();
        setPick((p) => (p + (e.key === "ArrowDown" ? 1 : options - 1)) % options);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        choose(pick);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setText((s) => s + " ");
        return;
      }
    }
    if (e.key === "ArrowUp" && !text) {
      const last = [...document.querySelectorAll<HTMLElement>("[data-message-id]")].pop();
      if (last) {
        e.preventDefault();
        last.focus();
        return;
      }
    }
    // On a phone Enter is a new line; the send button sends.
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing && width !== "narrow") {
      e.preventDefault();
      void submit();
    }
  };

  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = [...e.clipboardData.files].filter((f) => f.type.startsWith("image/"));
    if (files.length) {
      e.preventDefault();
      tray.add(files);
    }
  };

  return (
    <div className="@container relative mx-4 mb-4 rounded-lg border border-border-strong bg-surface focus-within:border-fg-muted">
      {popup ? (
        <ul
          role="listbox"
          aria-label={t(popup === "mention" ? "composer.mentions" : popup === "question" ? "composer.questions" : "composer.commandsLabel")}
          className="absolute inset-x-0 bottom-full z-(--z-overlay) mb-1 max-h-72 overflow-y-auto rounded-md border border-border bg-surface-raised p-1 shadow-overlay"
        >
          {popup === "mention"
            ? mentionList.map((a, i) => (
                <li
                  key={a.id}
                  role="option"
                  aria-selected={i === pick}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    choose(i);
                  }}
                  className={cn("flex h-9 cursor-pointer items-center gap-2 rounded-sm px-2", i === pick && "bg-surface-sunken")}
                >
                  <Avatar kind={a.kind} name={a.handle} emoji={a.avatar_emoji} size="sm" />
                  <span className="font-mono text-ui">@{a.handle}</span>
                  <span className="truncate text-meta text-fg-muted">{a.display_name}</span>
                </li>
              ))
            : popup === "question"
              ? questionList.map((q, i) => (
                  <li
                    key={q.id}
                    role="option"
                    aria-selected={i === pick}
                    onMouseDown={(e) => {
                      e.preventDefault();
                      choose(i);
                    }}
                    className={cn("flex cursor-pointer items-baseline gap-2 rounded-sm px-2 py-1.5", i === pick && "bg-surface-sunken")}
                  >
                    <span className="font-mono text-ui font-semibold">{q.formatted_number}</span>
                    <span className="font-mono text-meta text-fg-muted">@{q.asked_by_handle}</span>
                    <span className="min-w-0 flex-1 truncate text-meta text-fg-muted">{q.text}</span>
                  </li>
                ))
              : slash.map((c, i) => (
                <li
                  key={c.name}
                  role="option"
                  aria-selected={i === pick}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    choose(i);
                  }}
                  className={cn("flex cursor-pointer flex-col rounded-sm px-2 py-1.5", i === pick && "bg-surface-sunken")}
                >
                  <span className="font-mono text-ui">{c.usage}</span>
                  <span className="text-meta text-fg-muted">{t(`commands.${c.name}`)}</span>
                </li>
              ))}
        </ul>
      ) : null}
      <Tray tray={tray} />
      <textarea
        ref={area}
        aria-label={t("composer.label")}
        placeholder={placeholder}
        rows={2}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setCaret(e.target.selectionStart);
          setPick(0);
        }}
        onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
        onKeyDown={onKeyDown}
        onPaste={onPaste}
        className="block max-h-60 min-h-11 w-full resize-none bg-transparent px-3 pt-2.5 pb-1 outline-none placeholder:text-fg-subtle"
      />
      <div className="flex items-center gap-0.5 px-1.5 pb-1.5">
        <Menu>
          <MenuTrigger asChild>
            <IconButton label={t("composer.attach")} icon={<Plus className="size-4" />} />
          </MenuTrigger>
          <MenuContent align="start">
            <MenuItem icon={<Paperclip aria-hidden className="size-4" />} onSelect={() => fileInput.current?.click()}>
              {t("composer.uploadFile")}
            </MenuItem>
            <MenuItem
              icon={<CalendarClock aria-hidden className="size-4" />}
              onSelect={() =>
                openScheduleBuilder({
                  body: text,
                  channelId,
                  threadRootId,
                  onSaved: () => {
                    setText("");
                    setCaret(0);
                  },
                })
              }
            >
              {t("composer.schedule")}
            </MenuItem>
            {width === "narrow" ? (
              <MenuItem icon={<Camera aria-hidden className="size-4" />} onSelect={() => cameraInput.current?.click()}>
                {t("composer.photo")}
              </MenuItem>
            ) : null}
          </MenuContent>
        </Menu>
        <input ref={fileInput} type="file" multiple hidden onChange={(e) => (tray.add(e.target.files ?? []), (e.target.value = ""))} />
        <input ref={cameraInput} type="file" accept="image/*" capture="environment" hidden onChange={(e) => (tray.add(e.target.files ?? []), (e.target.value = ""))} />
        {onOpenFiles ? (
          <button type="button" onClick={onOpenFiles} className="inline-flex h-8 items-center gap-1.5 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg">
            <FolderOpen aria-hidden className="size-4" />
            {t("composer.files")}
          </button>
        ) : null}
        {onOpenTerminals ? (
          <button type="button" onClick={onOpenTerminals} className="inline-flex h-8 items-center gap-1.5 rounded-md px-2 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg">
            <SquareTerminal aria-hidden className="size-4" />
            {t("composer.terminal")}
          </button>
        ) : null}
        <span className="ml-2 hidden truncate text-meta text-fg-subtle @xl:inline">{t("composer.hint")}</span>
        <IconButton
          label={t("composer.send")}
          icon={<SendHorizontal className="size-4" />}
          onClick={() => void submit()}
          disabled={send.isPending}
          className="ml-auto bg-accent text-accent-fg hover:bg-accent-hover hover:text-accent-fg"
        />
      </div>
    </div>
  );
}
