import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { MessageSquare, NotebookPen, Search } from "lucide-react";
import { useActors } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { useNow } from "@/shared/hooks";
import { cn, relativeTime } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Button, EmptyState, Input, PageHeader, Skeleton } from "@/shared/ui";
import { openWindow } from "@/shared/windows";
import { useAllNotes, useThreadRoot } from "../api";
import { NoteCard } from "../components/NoteCard";
import { groupNotes, searchNotes } from "../model";

function ThreadTitle({ rootId }: { rootId: string }) {
  const { t } = useTranslation("notes");
  const root = useThreadRoot(rootId);
  return <>{root.data?.body.split("\n")[0] || t("untitled")}</>;
}

/** Jegyzetek (round 32, part 5): the agents' thread notebooks, read-only for humans. */
export default function NotesPage() {
  const { t, i18n } = useTranslation("notes");
  const locale = localeOf(i18n.language);
  const now = useNow();
  const actors = useActors();
  const [, dispatch] = usePanes();
  const agents = useMemo(
    () => [...(actors.data?.values() ?? [])].filter((a) => a.kind === "agent").sort((a, b) => a.handle.localeCompare(b.handle)),
    [actors.data],
  );
  const all = useAllNotes(agents.map((a) => a.id));
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState<{ agent: string; thread: string } | null>(null);
  const found = useMemo(() => searchNotes(all.notes, query), [all.notes, query]);
  const groups = useMemo(() => groupNotes(found, agents.map((a) => a.id)), [found, agents]);
  const current =
    groups.find((g) => g.agentId === selected?.agent)?.threads.find((x) => x.thread === selected?.thread) ??
    (selected ? undefined : groups[0]?.threads[0] && { ...groups[0].threads[0] });
  const currentAgent = selected?.agent ?? groups[0]?.agentId;

  return (
    <div className="flex h-full flex-col">
      <PageHeader icon={<NotebookPen className="size-5" />} title={t("title")} onPopOut={() => openWindow("notes")} popOutLabel={t("popOut")} />
      <div className="flex min-h-0 flex-1 max-md:flex-col">
        <aside className="flex w-72 shrink-0 flex-col gap-2 border-r border-border p-3 max-md:w-full max-md:border-r-0 max-md:border-b md:overflow-y-auto">
          <label className="relative">
            <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
            <Input aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} className="pl-8" />
          </label>
          {all.pending ? <Skeleton className="h-5 w-2/3" /> : null}
          {!all.pending && groups.length === 0 ? <p className="text-meta text-fg-subtle">{query ? t("noMatch") : t("empty")}</p> : null}
          {groups.map((g) => {
            const agent = actors.data?.get(g.agentId);
            return (
              <section key={g.agentId} aria-label={`@${agent?.handle}`}>
                <h2 className="px-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">@{agent?.handle}</h2>
                <ul className="mt-1 flex flex-col gap-px">
                  {g.threads.map((th) => {
                    const active = currentAgent === g.agentId && current?.thread === th.thread;
                    return (
                      <li key={th.thread}>
                        <button
                          type="button"
                          aria-current={active ? "true" : undefined}
                          onClick={() => setSelected({ agent: g.agentId, thread: th.thread })}
                          className={cn("flex w-full flex-col rounded-md px-2 py-1.5 text-left hover:bg-surface-sunken", active && "bg-accent-soft")}
                        >
                          <span className="truncate text-ui font-medium">
                            <ThreadTitle rootId={th.thread} />
                          </span>
                          <span className="text-meta text-fg-subtle">
                            {t("notes", { count: th.notes.length })} · {relativeTime(th.last, now, locale)}
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              </section>
            );
          })}
        </aside>
        <section aria-label={t("thread")} className="min-h-0 flex-1 overflow-y-auto p-4">
          {current ? (
            <div className="mx-auto flex max-w-3xl flex-col gap-3">
              <div className="flex items-center gap-2">
                <h2 className="min-w-0 flex-1 truncate text-title font-semibold">
                  <ThreadTitle rootId={current.thread} />
                </h2>
                <Button size="sm" icon={<MessageSquare className="size-4" />} onClick={() => dispatch({ type: "openThread", rootId: current.thread })}>
                  {t("openThread")}
                </Button>
              </div>
              {current.notes.map((n) => (
                <NoteCard key={n.id} note={n} agent={actors.data?.get(n.agent_id)} now={now} />
              ))}
              <p className="text-meta text-fg-subtle">{t("readOnly")}</p>
            </div>
          ) : (
            <EmptyState icon={<NotebookPen className="size-8" />} title={t("pick")} body={t("readOnly")} />
          )}
        </section>
      </div>
    </div>
  );
}
