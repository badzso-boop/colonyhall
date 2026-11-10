import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime } from "@/shared/lib";
import { Notice } from "@/shared/ui";
import { searchTone, type SearchStatus } from "../model";
import { StatusCard, StatusRow } from "./StatusCard";

type Backend = SearchStatus["embedder"];

/** The "Keresés" card of the status page (round 36a): the index, the embedding and the reranking server. */
export function SearchStatusCard({ search }: { search: SearchStatus }) {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const when = (at: string | undefined) => (at ? fullTime(at, i18n.language) : "–");
  const backend = (id: "embedder" | "reranker", b: Backend) => {
    const down = b.configured && !b.reachable;
    return (
      <div className="mt-1 flex flex-col gap-1 border-t border-border pt-1">
        <p className="font-semibold text-fg">{t(`status.search.${id}`)}</p>
        <StatusRow label={t("status.search.configured")} value={t(b.configured ? "settings.yes" : "settings.no")} />
        {b.configured ? (
          <>
            <StatusRow label={t("status.reachable")} value={<span className={cn(down && "text-warning")}>{t(b.reachable ? "settings.yes" : "settings.no")}</span>} />
            <StatusRow label={t("status.search.model")} value={b.model ?? "–"} />
            <StatusRow label={t("status.search.lastOk")} value={when(b.last_ok_at)} />
            {b.last_error ? <StatusRow label={t("status.search.lastError")} value={<span className="text-warning">{b.last_error}</span>} /> : null}
          </>
        ) : null}
      </div>
    );
  };
  return (
    <StatusCard title={t("status.search.title")} tone={searchTone(search)}>
      {search.model_mismatch ? <Notice tone="danger">{t("status.search.mismatch", { detail: search.model_mismatch })}</Notice> : null}
      <StatusRow label={t("status.search.chunks")} value={search.chunks.toLocaleString(locale)} />
      <StatusRow label={t("status.search.pending")} value={search.pending.toLocaleString(locale)} />
      <StatusRow label={t("status.search.lastEmbed")} value={when(search.last_embed_at)} />
      <StatusRow label={t("status.search.models")} value={search.models.length ? search.models.join(", ") : "–"} />
      {backend("embedder", search.embedder)}
      {backend("reranker", search.reranker)}
    </StatusCard>
  );
}
