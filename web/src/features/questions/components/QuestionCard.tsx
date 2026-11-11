import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Clock, MessageSquare } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime, hueVar, messageTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { Avatar, Badge, Button, Input, toast } from "@/shared/ui";
import { useActors } from "@/features/identity";
import { useAnswerQuestion } from "../api";
import { countdown, type Question } from "../model";

/**
 * One question (round 34, part 3): number, the asking agent, the
 * text, a link to its thread, the deadline countdown and default option;
 * the options as buttons and a free-text answer. The question text is the
 * agent's: shown as text, never interpreted.
 */
export function QuestionCard({ question, highlight, onNavigate, onAnswered }: { question: Question; highlight?: boolean; onNavigate?: () => void; onAnswered?: (q: Question) => void }) {
  const { t, i18n } = useTranslation("questions");
  const locale = localeOf(i18n.language);
  const now = useNow(1000);
  const actors = useActors();
  const answer = useAnswerQuestion();
  const [text, setText] = useState("");
  const asker = actors.data?.get(question.asked_by);
  const waiting = question.state === "open" || question.state === "escalated";
  const left = waiting ? countdown(question.deadline, now) : null;

  const send = async (value: string) => {
    const v = value.trim();
    if (!v) return;
    try {
      const q = await answer.mutateAsync({ id: question.id, answer: v });
      setText("");
      onAnswered?.(q);
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), { tone: "danger" });
    }
  };

  return (
    <article
      data-question={question.formatted_number}
      aria-label={question.formatted_number}
      className={cn("flex flex-col gap-2 rounded-lg border bg-surface-raised p-3", highlight ? "border-accent" : "border-border")}
    >
      <header className="flex flex-wrap items-center gap-x-2 gap-y-1 text-meta">
        <span className="font-mono font-semibold text-fg">{question.formatted_number}</span>
        <Avatar kind="agent" name={question.asked_by_handle} emoji={asker?.avatar_emoji} size="sm" ringColor={hueVar(asker?.effective_color)} />
        <span className="font-mono text-fg-muted">@{question.asked_by_handle}</span>
        {question.state === "escalated" ? <Badge tone="warning">{t("state.escalated")}</Badge> : null}
        {question.state === "answered" ? <Badge tone="success">{t("state.answered")}</Badge> : null}
        {question.state === "expired" ? <Badge>{t("state.expired")}</Badge> : null}
        {question.state === "withdrawn" ? <Badge>{t("state.withdrawn")}</Badge> : null}
        <time className="text-fg-subtle" dateTime={question.created_at} title={fullTime(question.created_at, locale)}>
          {messageTime(question.created_at, now, locale)}
        </time>
        {left ? (
          <span className={cn("tabular ml-auto inline-flex items-center gap-1", left.overdue ? "text-danger" : "text-fg-muted")} title={question.deadline ? fullTime(question.deadline, locale) : undefined}>
            <Clock aria-hidden className="size-3.5" />
            {left.overdue ? t("overdue") : t("deadline", { left: left.text })}
          </span>
        ) : null}
      </header>
      <p className="text-body font-medium whitespace-pre-wrap">{question.text}</p>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-meta text-fg-muted">
        <Link to={`/c/${question.channel_id}?thread=${question.thread_root_id}`} onClick={onNavigate} className="inline-flex items-center gap-1 text-accent-text hover:underline">
          <MessageSquare aria-hidden className="size-3.5" />
          {t("thread", { channel: question.channel_name })}
        </Link>
        {question.default_option && waiting ? <span>{t("default", { option: question.default_option })}</span> : null}
      </div>
      {waiting ? (
        <>
          {question.options.length ? (
            <div className="flex flex-wrap gap-1.5" role="group" aria-label={t("options")}>
              {question.options.map((o) => (
                <Button key={o} size="sm" variant={o === question.default_option ? "primary" : "secondary"} disabled={answer.isPending} onClick={() => void send(o)}>
                  {o}
                </Button>
              ))}
            </div>
          ) : null}
          <form
            className="flex gap-1.5"
            onSubmit={(e) => {
              e.preventDefault();
              void send(text);
            }}
          >
            <Input value={text} onChange={(e) => setText(e.target.value)} placeholder={t("freeText")} aria-label={t("freeTextLabel", { q: question.formatted_number })} />
            <Button type="submit" size="sm" loading={answer.isPending} disabled={!text.trim()}>
              {t("send")}
            </Button>
          </form>
        </>
      ) : question.answer ? (
        <p className="rounded-md bg-surface-sunken px-2.5 py-1.5 text-ui">
          <span className="text-fg-muted">
            {t("answeredBy", { who: question.answered_by_handle ? `@${question.answered_by_handle}` : "?", when: question.answered_at ? messageTime(question.answered_at, now, locale) : "" })}
          </span>{" "}
          <span className="whitespace-pre-wrap">{question.answer}</span>
        </p>
      ) : null}
    </article>
  );
}
