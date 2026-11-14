// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Flag, Pin } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { messageHref, plainText, relativeTime } from "@/shared/lib";
import { Popover, PopoverContent, PopoverTrigger, Skeleton } from "@/shared/ui";
import { usePins } from "../api";
import type { Actor } from "../model";

/** The header's "Kitűzve" (round 55, part D): the count, and a list that jumps to each pinned message. */
export function PinsButton({ channelId, actors }: { channelId: string; actors: Map<string, Actor> }) {
  const { t, i18n } = useTranslation("conversation");
  const locale = localeOf(i18n.language);
  const pins = usePins(channelId);
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const count = pins.data?.length ?? 0;
  if (!count) return null;
  const now = new Date();
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={t("pins.open", { count })}
          className="flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-meta text-fg-muted hover:bg-surface-sunken hover:text-fg"
        >
          <Pin aria-hidden className="size-3.5" />
          <span className="tabular-nums">{count}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent label={t("pins.title")} align="end" className="w-96 p-1">
        <p className="px-2 py-1.5 text-meta font-semibold text-fg-muted">{t("pins.title")}</p>
        {!pins.data ? (
          <Skeleton className="m-2 h-8" />
        ) : (
          <ul className="max-h-[60dvh] overflow-y-auto">
            {pins.data.map((p) => {
              const author = actors.get(p.message.author_id);
              return (
                <li key={p.message.id}>
                  <button
                    type="button"
                    onClick={() => {
                      setOpen(false);
                      navigate(messageHref(p.message));
                    }}
                    className="flex w-full flex-col gap-0.5 rounded-sm px-2 py-1.5 text-left hover:bg-surface-sunken"
                  >
                    <span className="flex items-center gap-2 text-meta text-fg-muted">
                      <span className="font-semibold text-fg">{author ? author.display_name || author.handle : "?"}</span>
                      {p.message.important ? (
                        <span className="inline-flex items-center gap-0.5 text-warning">
                          <Flag aria-hidden className="size-3" />
                          {t("pins.important")}
                        </span>
                      ) : null}
                      <span className="ml-auto text-fg-subtle">{relativeTime(p.message.created_at, now, locale)}</span>
                    </span>
                    <span className="line-clamp-2 text-ui">{plainText(p.message.body)}</span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </PopoverContent>
    </Popover>
  );
}
