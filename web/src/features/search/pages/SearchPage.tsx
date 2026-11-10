import type { FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { Search, SearchX, TriangleAlert } from "lucide-react";
import { cn } from "@/shared/lib";
import { Button, EmptyState, Input, Notice, PageHeader, Skeleton } from "@/shared/ui";
import { useSearch } from "../api";
import { HitCard } from "../components/HitCard";
import { DEFAULT_MODE, groupHits, isSearchMode, searchModes, searchParams, type SearchMode } from "../model";

/** Keresés (/search?q=…&mode=…): hybrid search over messages, wiki, decisions and summaries (round 36a). */
export default function SearchPage() {
  const { t } = useTranslation("search");
  const [params, setParams] = useSearchParams();
  const q = (params.get("q") ?? "").trim();
  const rawMode = params.get("mode");
  const mode: SearchMode = isSearchMode(rawMode) ? rawMode : DEFAULT_MODE;
  const result = useSearch(q, mode);
  const go = (nextQ: string, nextMode: SearchMode) => setParams(searchParams(nextQ, nextMode));
  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    go(String(new FormData(e.currentTarget).get("q") ?? ""), mode);
  };
  const now = new Date();
  const data = q ? result.data : undefined;
  const groups = data ? groupHits(data.hits) : [];
  return (
    <div className="flex h-full flex-col">
      <PageHeader icon={<Search className="size-5" />} title={t("title")} />
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto flex max-w-3xl flex-col gap-3 p-4">
          {/* The key resets the field when the URL's query changes (the palette, back button). */}
          <form key={q} role="search" onSubmit={submit} className="flex flex-wrap items-center gap-2">
            <label className="relative min-w-0 flex-1 basis-64">
              <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
              <Input type="search" name="q" autoFocus aria-label={t("field")} placeholder={t("field")} defaultValue={q} className="pl-8" />
            </label>
            <Button type="submit" variant="primary">
              {t("submit")}
            </Button>
          </form>
          <div role="radiogroup" aria-label={t("mode.label")} className="flex self-start rounded-md border border-border p-0.5">
            {searchModes.map((m) => (
              <button
                key={m}
                type="button"
                role="radio"
                aria-checked={mode === m}
                onClick={() => go(q, m)}
                className={cn("h-7 rounded px-3 text-ui text-fg-muted hover:text-fg", mode === m && "bg-accent-soft font-semibold text-fg")}
              >
                {t(`mode.${m}`)}
              </button>
            ))}
          </div>
          {!q ? <p className="text-meta text-fg-subtle">{t("hint")}</p> : null}
          {q && result.isError ? <Notice tone="danger">{t("failed", { message: result.error.message })}</Notice> : null}
          {q && result.isPending ? (
            <div aria-busy="true" className="flex flex-col gap-2">
              <p className="text-meta text-fg-subtle">{t("loading")}</p>
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : null}
          {data?.notice ? (
            <Notice tone="warning" icon={<TriangleAlert aria-hidden className="size-4 text-warning" />}>
              {data.notice}
            </Notice>
          ) : null}
          {data && data.hits.length === 0 ? <EmptyState icon={<SearchX className="size-8" />} title={t("empty", { q })} body={t("emptyBody")} /> : null}
          <div className={cn("flex flex-col gap-4", result.isFetching && !result.isPending && "opacity-70")}>
            {groups.map((g) => (
              <section key={g.kind} aria-labelledby={`search-${g.kind}`} className="flex flex-col gap-2">
                <h2 id={`search-${g.kind}`} className="flex items-baseline gap-2 font-semibold">
                  {t(`kinds.${g.kind}`)}
                  <span className="text-meta font-normal text-fg-subtle">{t("count", { count: g.hits.length })}</span>
                </h2>
                {g.hits.map((h) => (
                  <HitCard key={`${h.kind}-${h.source_id}-${h.part}`} hit={h} now={now} />
                ))}
              </section>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
