import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { AlarmClock, Bookmark, Check, RotateCcw, Trash2 } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime, messageHref, plainText, relativeTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { Badge, Button, EmptyState, IconButton, PageHeader, Skeleton, toast } from "@/shared/ui";
import { useDeleteSaved, useSaved, useUpdateSaved, type SavedFilter, type SavedItem } from "../api";
import { ReminderDialog } from "../components/ReminderDialog";

const filters: SavedFilter[] = ["all", "reminders", "done"];

function Row({ item }: { item: SavedItem }) {
  const { t, i18n } = useTranslation("saved");
  const locale = localeOf(i18n.language);
  const now = useNow(60_000);
  const actors = useActors();
  const update = useUpdateSaved();
  const remove = useDeleteSaved();
  const [reminding, setReminding] = useState(false);
  const m = item.message;
  const author = m ? actors.data?.get(m.author_id) : undefined;
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  const place = m ? (m.channel_kind === "dm" ? t("dm") : `# ${m.channel_name}`) : null;
  const body = (
    <>
      <p className="flex flex-wrap items-baseline gap-x-2 text-meta text-fg-muted">
        {place ? <span className="font-semibold">{place}</span> : null}
        {author ? <span>{author.display_name || author.handle}</span> : null}
        {m?.thread_root_id ? <span>{t("inThread")}</span> : null}
        <span className="ml-auto text-fg-subtle">{relativeTime(item.created_at, now, locale)}</span>
      </p>
      {item.note ? <p className="mt-1 text-ui font-semibold">{item.note}</p> : null}
      {m ? (
        <p className="mt-1 line-clamp-3 text-ui break-words text-fg">{m.retracted ? t("retracted") : plainText(m.body)}</p>
      ) : item.message_id ? (
        <p className="mt-1 text-ui text-fg-subtle italic">{t("unavailable")}</p>
      ) : null}
    </>
  );
  return (
    <li className={cn("flex items-start gap-3 rounded-lg border border-border bg-surface px-4 py-3", item.done_at && "opacity-70")}>
      <div className="min-w-0 flex-1">
        {m ? (
          <Link to={messageHref({ channel_id: m.channel_id, id: m.id, thread_root_id: m.thread_root_id })} className="block hover:underline-offset-2">
            {body}
          </Link>
        ) : (
          body
        )}
        {item.remind_at ? (
          <p className="mt-1.5 flex items-center gap-1.5 text-meta">
            <AlarmClock aria-hidden className="size-3.5 text-fg-subtle" />
            <time dateTime={item.remind_at}>{fullTime(item.remind_at, locale)}</time>
            {item.reminded_at ? <Badge tone="neutral">{t("reminded")}</Badge> : null}
          </p>
        ) : null}
      </div>
      <div className="flex shrink-0 gap-0.5">
        <IconButton size="sm" label={t("remindMe")} icon={<AlarmClock className="size-4" />} onClick={() => setReminding(true)} />
        {item.done_at ? (
          <IconButton size="sm" label={t("undone")} icon={<RotateCcw className="size-4" />} onClick={() => update.mutate({ id: item.id, done: false }, { onError: fail })} />
        ) : (
          <IconButton size="sm" label={t("markDone")} icon={<Check className="size-4" />} onClick={() => update.mutate({ id: item.id, done: true }, { onError: fail })} />
        )}
        <IconButton size="sm" label={t("remove")} icon={<Trash2 className="size-4" />} onClick={() => remove.mutate(item.id, { onError: fail })} />
      </div>
      {reminding ? <ReminderDialog itemId={item.id} onClose={() => setReminding(false)} /> : null}
    </li>
  );
}

/** "Mentett" (round 55, part E): the viewer's saved messages and reminders. */
export default function SavedPage() {
  const { t } = useTranslation("saved");
  const human = useSession().data?.actor.kind === "human";
  const [filter, setFilter] = useState<SavedFilter>("all");
  const [newReminder, setNewReminder] = useState(false);
  const saved = useSaved(filter, human);
  const list = saved.data?.pages.flatMap((p) => p.items) ?? [];
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title={t("title")}
        actions={
          <Button size="sm" variant="ghost" onClick={() => setNewReminder(true)}>
            <AlarmClock aria-hidden className="size-4" />
            {t("newReminder")}
          </Button>
        }
      />
      <div role="radiogroup" aria-label={t("filter")} className="flex gap-1 px-4 pt-3">
        {filters.map((f) => (
          <button
            key={f}
            type="button"
            role="radio"
            aria-checked={filter === f}
            onClick={() => setFilter(f)}
            className={cn("rounded-full border px-3 py-1 text-meta", filter === f ? "border-accent bg-accent-soft font-semibold" : "border-border text-fg-muted")}
          >
            {t(`filters.${f}`)}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {!saved.data ? (
          <Skeleton className="h-24 w-full" />
        ) : list.length === 0 ? (
          <EmptyState icon={<Bookmark className="size-6" />} title={t(`empty.${filter}`)} body={t("emptyBody")} />
        ) : (
          <ul className="mx-auto flex max-w-3xl flex-col gap-2">
            {list.map((it) => (
              <Row key={it.id} item={it} />
            ))}
            {saved.hasNextPage ? (
              <Button variant="ghost" loading={saved.isFetchingNextPage} onClick={() => void saved.fetchNextPage()}>
                {t("loadMore")}
              </Button>
            ) : null}
          </ul>
        )}
      </div>
      {newReminder ? <ReminderDialog onClose={() => setNewReminder(false)} /> : null}
    </div>
  );
}
