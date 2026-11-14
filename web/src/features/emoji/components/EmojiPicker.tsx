import { useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { Clock, Flag, Hash, Lightbulb, PawPrint, Plane, Smile, Star, Trophy, Users, Utensils, type LucideIcon } from "lucide-react";
import { Input, Skeleton } from "@/shared/ui";
import { useEmojiUsage, useRecordEmoji } from "../api";
import { loadEmoji } from "../load";
import { emojiName, groups, searchEmoji, type Emoji } from "../model";

const groupIcon: Record<number, LucideIcon> = { 0: Smile, 1: Users, 3: PawPrint, 4: Utensils, 5: Plane, 6: Trophy, 7: Lightbulb, 8: Hash, 9: Flag };

function Grid({ list, language, onPick }: { list: Emoji[]; language: string; onPick: (e: Emoji) => void }) {
  return (
    <div className="grid grid-cols-8 gap-0.5">
      {list.map((e) => {
        const name = emojiName(e, language);
        return (
          <button
            key={e.char}
            type="button"
            aria-label={name}
            title={e.code ? `${name} :${e.code}:` : name}
            onClick={() => onPick(e)}
            className="grid size-9 place-items-center rounded-sm text-xl leading-none hover:bg-surface-sunken focus-visible:bg-surface-sunken"
          >
            {e.char}
          </button>
        );
      })}
    </div>
  );
}

/**
 * The emoji picker (round 55, part C): search by name or keyword in
 * Hungarian and English, the person's recent and frequent emoji first,
 * then the Unicode groups. It is the lazy chunk with the emoji data.
 */
export default function EmojiPicker({ onPick }: { onPick: (char: string) => void }) {
  const { t, i18n } = useTranslation("emoji");
  const data = useQuery({ queryKey: ["emoji", "data"], queryFn: loadEmoji, staleTime: Infinity, gcTime: Infinity });
  const usage = useEmojiUsage(true);
  const record = useRecordEmoji();
  const [query, setQuery] = useState("");
  const scroller = useRef<HTMLDivElement>(null);
  const list = data.data;

  const byChar = useMemo(() => new Map((list ?? []).map((e) => [e.char, e])), [list]);
  const results = useMemo(() => (list && query.trim() ? searchEmoji(list, query) : null), [list, query]);
  const known = (chars: string[] | undefined) => (chars ?? []).map((c) => byChar.get(c)).filter((e): e is Emoji => e !== undefined);
  const recent = known(usage.data?.recent);
  const frequent = known(usage.data?.frequent);

  const pick = (e: Emoji) => {
    record.mutate(e.char);
    onPick(e.char);
  };
  const onKeyDown = (ev: KeyboardEvent<HTMLInputElement>) => {
    if (ev.key === "Enter" && results?.[0]) {
      ev.preventDefault();
      pick(results[0]);
    }
  };
  const jump = (id: string) => scroller.current?.querySelector(`[data-group="${id}"]`)?.scrollIntoView({ block: "start" });

  const section = (id: string, title: string, items: Emoji[]) =>
    items.length ? (
      <section key={id} data-group={id} aria-label={title} className="pb-2">
        <h3 className="sticky top-0 bg-surface-raised py-1 text-meta font-medium text-fg-muted">{title}</h3>
        <Grid list={items} language={i18n.language} onPick={pick} />
      </section>
    ) : null;

  return (
    <div className="flex flex-col">
      <div className="p-2 pb-1">
        <Input autoFocus type="search" aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={onKeyDown} />
      </div>
      {!results ? (
        <nav aria-label={t("groupsLabel")} className="flex gap-0.5 border-b border-border px-2 pb-1">
          {[
            ...(recent.length ? [{ id: "recent", Icon: Clock, title: t("recent") }] : []),
            ...(frequent.length ? [{ id: "frequent", Icon: Star, title: t("frequent") }] : []),
            ...groups.map((g) => ({ id: String(g), Icon: groupIcon[g]!, title: t(`groups.${g}`) })),
          ].map(({ id, Icon, title }) => (
            <button key={id} type="button" aria-label={title} title={title} onClick={() => jump(id)} className="grid size-7 place-items-center rounded-sm text-fg-muted hover:bg-surface-sunken hover:text-fg">
              <Icon aria-hidden className="size-4" />
            </button>
          ))}
        </nav>
      ) : null}
      <div ref={scroller} className="h-72 overflow-y-auto px-2 pt-1">
        {!list ? (
          <Skeleton className="m-2 h-40" />
        ) : results ? (
          results.length ? (
            <Grid list={results} language={i18n.language} onPick={pick} />
          ) : (
            <p className="p-4 text-center text-ui text-fg-muted">{t("none")}</p>
          )
        ) : (
          <>
            {section("recent", t("recent"), recent)}
            {section("frequent", t("frequent"), frequent)}
            {groups.map((g) => section(String(g), t(`groups.${g}`), list.filter((e) => e.group === g)))}
          </>
        )}
      </div>
    </div>
  );
}

