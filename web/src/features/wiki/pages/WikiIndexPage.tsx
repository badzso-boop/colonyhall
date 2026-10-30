import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { BookOpen, Plus, RotateCcw, Search } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { useLocalStorage } from "@/shared/hooks";
import { cn, fullTime, relativeTime } from "@/shared/lib";
import { Badge, Button, EmptyState, Input, PageHeader, Select, Skeleton } from "@/shared/ui";
import { seenTopics, useRestoreTopic, useWikiIndex, type WikiIndex } from "../api";
import { ActorAvatar } from "../components/People";
import { NewTopicDialog } from "../components/NewTopicDialog";
import { tagCounts } from "../posts";

type Sort = "activity" | "created" | "title";

function sortTopics(list: WikiIndex[], by: Sort): WikiIndex[] {
  const out = [...list];
  if (by === "title") return out.sort((a, b) => a.title.localeCompare(b.title));
  const key = by === "created" ? "created_at" : "updated_at";
  return out.sort((a, b) => b[key].localeCompare(a[key]));
}

/** Wiki index as a forum (docs/handoff/32, part 7): tags as categories, topic rows, sort, search. */
export default function WikiIndexPage() {
  const { t, i18n } = useTranslation("wiki");
  const locale = localeOf(i18n.language);
  const actors = useActors();
  const session = useSession();
  const human = session.data?.actor.kind === "human";
  const [q, setQ] = useState("");
  const [deleted, setDeleted] = useState(false);
  const [tag, setTag] = useState<string | null>(null);
  const [sort, setSort] = useLocalStorage<Sort>("colony.wiki.sort", "activity");
  const [creating, setCreating] = useState(false);
  const index = useWikiIndex(q.trim(), deleted && human);
  const restore = useRestoreTopic();
  const seen = seenTopics();
  const now = new Date();

  const all = useMemo(() => (index.data ?? []).filter((p) => (deleted ? p.deleted_at : !p.deleted_at)), [index.data, deleted]);
  const tags = useMemo(() => tagCounts(all), [all]);
  const topics = useMemo(() => sortTopics(tag ? all.filter((p) => p.tags.includes(tag)) : all, sort), [all, tag, sort]);

  const tagButton = (value: string | null, label: string, count: number) => (
    <button
      key={value ?? ""}
      type="button"
      aria-pressed={tag === value}
      onClick={() => setTag(value)}
      className={cn(
        "flex items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-ui hover:bg-surface-sunken max-md:shrink-0 max-md:rounded-full max-md:border max-md:border-border",
        tag === value && "bg-accent-soft font-semibold",
      )}
    >
      <span className="truncate">{label}</span>
      <span className="text-meta text-fg-subtle tabular-nums">{count}</span>
    </button>
  );

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<BookOpen className="size-5" />}
        title={t("title")}
        actions={
          <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
            {t("newTopic")}
          </Button>
        }
      />
      <div className="flex min-h-0 flex-1 max-md:flex-col">
        <nav aria-label={t("tags")} className="flex w-52 shrink-0 flex-col gap-px border-r border-border p-3 max-md:w-full max-md:flex-row max-md:gap-1.5 max-md:overflow-x-auto max-md:border-r-0 max-md:border-b md:overflow-y-auto">
          {tagButton(null, t("all"), all.length)}
          {tags.map(([x, n]) => tagButton(x, `#${x}`, n))}
        </nav>
        <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
          <div className="flex flex-wrap items-center gap-2">
            <label className="relative min-w-48 flex-1">
              <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
              <Input type="search" aria-label={t("search")} placeholder={t("search")} value={q} onChange={(e) => setQ(e.target.value)} className="pl-8" />
            </label>
            <Select aria-label={t("sort")} value={sort} onChange={(e) => setSort(e.target.value as Sort)} className="w-44">
              {(["activity", "created", "title"] as const).map((s) => (
                <option key={s} value={s}>
                  {t(`sorts.${s}`)}
                </option>
              ))}
            </Select>
            {human ? (
              <button
                type="button"
                aria-pressed={deleted}
                onClick={() => setDeleted(!deleted)}
                className={cn("h-8 rounded-md border px-2.5 text-ui", deleted ? "border-accent bg-accent-soft font-semibold" : "border-border text-fg-muted")}
              >
                {t("showDeleted")}
              </button>
            ) : null}
          </div>
          {index.isPending ? <Skeleton className="h-16 w-full" /> : null}
          {index.data && topics.length === 0 ? <EmptyState icon={<BookOpen className="size-8" />} title={q || tag ? t("noMatch") : t("empty")} /> : null}
          <ul className="divide-y divide-border rounded-lg border border-border">
            {topics.map((p) => {
              const author = p.created_by ? actors.data?.get(p.created_by) : undefined;
              const fresh = !p.deleted_at && seen[p.slug] !== undefined && p.updated_at > seen[p.slug]!;
              // Never opened: new only while it is a week old.
              const unseen = !p.deleted_at && seen[p.slug] === undefined && now.getTime() - new Date(p.created_at).getTime() < 7 * 86_400_000;
              return (
                <li key={p.slug} className="flex items-start gap-3 p-3">
                  <ActorAvatar actor={author} size="md" />
                  <div className="min-w-0 flex-1">
                    <p className="flex items-center gap-2">
                      <Link to={`/wiki/${p.slug}`} className="truncate font-semibold hover:underline">
                        {p.title}
                      </Link>
                      {fresh || unseen ? <Badge tone="accent">{t("new")}</Badge> : null}
                      {p.deleted_at ? <Badge tone="danger">{t("deleted")}</Badge> : null}
                    </p>
                    <p className="line-clamp-2 text-ui text-fg-muted">{p.excerpt}</p>
                    <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-meta text-fg-subtle">
                      {p.tags.map((x) => (
                        <button key={x} type="button" onClick={() => setTag(x)} className="hover:text-fg">
                          #{x}
                        </button>
                      ))}
                      {author ? <span>@{author.handle}</span> : null}
                    </p>
                  </div>
                  <div className="flex shrink-0 flex-col items-end gap-1 text-meta text-fg-subtle">
                    <span className="flex -space-x-1.5">
                      {(p.participant_ids ?? []).slice(0, 3).map((id) => (
                        <ActorAvatar key={id} actor={actors.data?.get(id)} />
                      ))}
                    </span>
                    <span className="tabular-nums">{t("posts", { count: p.post_count ?? 1 })}</span>
                    <time dateTime={p.updated_at} title={fullTime(p.updated_at, locale)}>
                      {relativeTime(p.updated_at, now, locale)}
                    </time>
                    {p.deleted_at && human ? (
                      <Button size="sm" icon={<RotateCcw className="size-3.5" />} loading={restore.isPending} onClick={() => restore.mutate(p.slug)}>
                        {t("restore")}
                      </Button>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      </div>
      {creating ? <NewTopicDialog open onOpenChange={setCreating} knownTags={tags.map(([x]) => x)} /> : null}
    </div>
  );
}
