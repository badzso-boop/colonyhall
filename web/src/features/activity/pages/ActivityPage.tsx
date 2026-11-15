// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { AlarmClock, AtSign, Bell, CheckCheck, Eye, Flag, HelpCircle, MessageSquareReply, ShieldQuestion, SmilePlus } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { cn, plainText, relativeTime } from "@/shared/lib";
import { useNow } from "@/shared/hooks";
import { Button, EmptyState, PageHeader, Skeleton, toast } from "@/shared/ui";
import { useActivity, useMarkActivityRead, type ActivityItem, type ActivityKind } from "../api";
import { activityKinds, activityTarget } from "../model";

const kindIcon: Record<ActivityKind, typeof Bell> = {
  mention: AtSign,
  thread_reply: MessageSquareReply,
  reaction: SmilePlus,
  approval: ShieldQuestion,
  question: HelpCircle,
  important: Flag,
  reminder: AlarmClock,
  operator_access: Eye,
};

function Row({ item }: { item: ActivityItem }) {
  const { t, i18n } = useTranslation("activity");
  const locale = localeOf(i18n.language);
  const now = useNow(60_000);
  const actors = useActors();
  const navigate = useNavigate();
  const mark = useMarkActivityRead();
  const Icon = kindIcon[item.kind];
  const actor = item.actor_id ? actors.data?.get(item.actor_id) : undefined;
  const who = actor ? (actor.kind === "agent" ? `@${actor.handle}` : actor.display_name || actor.handle) : t("someone");
  const place = item.channel_name ? (item.channel_kind === "dm" ? t("dm") : `#${item.channel_name}`) : null;
  const target = activityTarget(item);
  const open = () => {
    if (!item.read) mark.mutate({ item_ids: [item.id] });
    if (target) navigate(target);
  };
  return (
    <li>
      <button
        type="button"
        onClick={open}
        disabled={!target && item.read}
        className={cn(
          "flex w-full items-start gap-3 rounded-lg border px-4 py-3 text-left",
          item.read ? "border-border bg-surface" : "border-accent/40 bg-accent-soft",
          target && "hover:border-border-strong",
        )}
      >
        <Icon aria-hidden className={cn("mt-0.5 size-4 shrink-0", item.kind === "important" || item.kind === "operator_access" ? "text-warning" : "text-fg-subtle")} />
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-baseline gap-x-2 text-meta text-fg-muted">
            <span className={cn("text-ui", !item.read && "font-semibold text-fg")}>{t(`kinds.${item.kind}`, { who, emoji: item.emoji ?? "" })}</span>
            {place ? <span>{place}</span> : null}
            <span className="ml-auto text-fg-subtle">{relativeTime(item.created_at, now, locale)}</span>
          </span>
          {item.excerpt ? <span className="mt-1 line-clamp-2 block text-ui text-fg">{plainText(item.excerpt)}</span> : null}
          {!item.available && item.kind !== "operator_access" ? <span className="mt-1 block text-meta text-fg-subtle italic">{t("unavailable")}</span> : null}
        </span>
        {!item.read ? <span className="mt-1.5 size-2 shrink-0 rounded-full bg-accent" aria-label={t("unread")} /> : null}
      </button>
    </li>
  );
}

/** "Tevékenység" (round 55, part H): what concerns the viewer, newest first. */
export default function ActivityPage() {
  const { t } = useTranslation("activity");
  const human = useSession().data?.actor.kind === "human";
  const [kind, setKind] = useState<ActivityKind | null>(null);
  const [unreadOnly, setUnreadOnly] = useState(false);
  const feed = useActivity(kind, unreadOnly, human);
  const markAll = useMarkActivityRead();
  const list = feed.data?.pages.flatMap((p) => p.items) ?? [];
  const unread = feed.data?.pages[0]?.unread_count ?? 0;
  const chip = (active: boolean) => cn("rounded-full border px-3 py-1 text-meta", active ? "border-accent bg-accent-soft font-semibold" : "border-border text-fg-muted");
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title={t("title")}
        actions={
          <Button
            size="sm"
            variant="ghost"
            disabled={unread === 0}
            loading={markAll.isPending}
            onClick={() => markAll.mutate({}, { onError: (e) => toast(e.message, { tone: "danger" }) })}
          >
            <CheckCheck aria-hidden className="size-4" />
            {t("markAll")}
          </Button>
        }
      />
      <div className="flex flex-wrap gap-1 px-4 pt-3">
        <div role="radiogroup" aria-label={t("filter")} className="flex flex-wrap gap-1">
          {[null, ...activityKinds].map((k) => (
            <button key={k ?? "all"} type="button" role="radio" aria-checked={kind === k} onClick={() => setKind(k)} className={chip(kind === k)}>
              {t(k ? `filters.${k}` : "filters.all")}
            </button>
          ))}
        </div>
        <button type="button" aria-pressed={unreadOnly} onClick={() => setUnreadOnly(!unreadOnly)} className={cn(chip(unreadOnly), "ml-auto")}>
          {t("unreadOnly")}
        </button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {!feed.data ? (
          <Skeleton className="h-24 w-full" />
        ) : list.length === 0 ? (
          <EmptyState icon={<Bell className="size-6" />} title={t("empty")} body={t("emptyBody")} />
        ) : (
          <ul className="mx-auto flex max-w-3xl flex-col gap-2">
            {list.map((it) => (
              <Row key={it.id} item={it} />
            ))}
            {feed.hasNextPage ? (
              <Button variant="ghost" loading={feed.isFetchingNextPage} onClick={() => void feed.fetchNextPage()}>
                {t("loadMore")}
              </Button>
            ) : null}
          </ul>
        )}
      </div>
    </div>
  );
}
