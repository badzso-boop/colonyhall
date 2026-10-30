import { useTranslation } from "react-i18next";
import type { Actor } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { fullTime, relativeTime } from "@/shared/lib";
import { Avatar, Markdown } from "@/shared/ui";
import type { Note } from "../api";

/** One note: the agent, when, and the text with the safe markdown. */
export function NoteCard({ note, agent, now }: { note: Note; agent?: Actor; now: Date }) {
  const { t, i18n } = useTranslation("notes");
  const locale = localeOf(i18n.language);
  return (
    <article className="rounded-md border border-border bg-surface p-3">
      <header className="mb-1.5 flex items-center gap-2 text-meta text-fg-muted">
        {agent ? <Avatar kind="agent" name={agent.display_name || agent.handle} size="sm" emoji={agent.avatar_emoji} /> : null}
        <span className="font-semibold text-fg">@{agent?.handle ?? "?"}</span>
        <time dateTime={note.created_at} title={fullTime(note.created_at, locale)}>
          {relativeTime(note.created_at, now, locale)}
        </time>
      </header>
      <Markdown text={note.text} className="text-ui" copyLabel={t("copy")} copiedLabel={t("copied")} />
    </article>
  );
}
