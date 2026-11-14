import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { PencilLine, Trash2 } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { relativeTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { EmptyState, IconButton, PageHeader, Skeleton, toast } from "@/shared/ui";
import { useChannels, useDrafts, useDraftWriter, type Draft } from "../api";
import { draftKey, writeLocalDraft } from "../drafts";
import { conversationTitle } from "../model";

function placeLink(d: Draft): string {
  return d.thread_root_id ? `/c/${d.channel_id}?thread=${d.thread_root_id}` : `/c/${d.channel_id}`;
}

/** "Piszkozatok" (round 55, part A): every unsent message of the viewer, newest first. */
export default function DraftsPage() {
  const { t, i18n } = useTranslation("conversation");
  const locale = localeOf(i18n.language);
  const now = useNow(60_000);
  const session = useSession();
  const human = session.data?.actor.kind === "human";
  const drafts = useDrafts(human);
  const channels = useChannels();
  const actors = useActors();
  const writer = useDraftWriter();
  const remove = (d: Draft) => {
    writeLocalDraft(draftKey(d.channel_id, d.thread_root_id), undefined);
    writer.remove(d.channel_id, d.thread_root_id).catch((e: unknown) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }));
  };
  const list = drafts.data ?? [];
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader title={t("drafts.title")} />
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {!drafts.data || !channels.data || !actors.data || !session.data ? (
          <Skeleton className="h-24 w-full" />
        ) : list.length === 0 ? (
          <EmptyState icon={<PencilLine className="size-6" />} title={t("drafts.none")} body={t("drafts.noneBody")} />
        ) : (
          <ul className="mx-auto flex max-w-3xl flex-col gap-2">
            {list.map((d) => {
              const channel = channels.data.find((c) => c.id === d.channel_id);
              const place = channel ? (channel.kind === "dm" ? conversationTitle(channel, actors.data, session.data!.actor.id) : `# ${channel.name}`) : "…";
              const files = d.attachments.length;
              return (
                <li key={draftKey(d.channel_id, d.thread_root_id)} className="group flex items-start gap-3 rounded-lg border border-border bg-surface px-4 py-3">
                  <Link to={placeLink(d)} className="min-w-0 flex-1">
                    <p className="flex items-baseline gap-2 text-meta text-fg-muted">
                      <span className="font-semibold">{place}</span>
                      {d.thread_root_id ? <span>{t("drafts.inThread")}</span> : null}
                      <span className="ml-auto text-fg-subtle">{relativeTime(d.updated_at, now, locale)}</span>
                    </p>
                    <p className="mt-1 line-clamp-2 text-ui break-words whitespace-pre-wrap">{d.body.trim() || t("drafts.onlyFiles", { count: files })}</p>
                    {files > 0 && d.body.trim() ? <p className="mt-1 text-meta text-fg-subtle">{t("drafts.files", { count: files })}</p> : null}
                  </Link>
                  <IconButton label={t("drafts.delete")} icon={<Trash2 className="size-4" />} onClick={() => remove(d)} />
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
