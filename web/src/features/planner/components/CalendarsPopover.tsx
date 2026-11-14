// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { localeOf } from "@/shared/i18n";
import { CalendarDays, RefreshCw, Trash2 } from "lucide-react";
import { relativeTime } from "@/shared/lib";
import { Button, Checkbox, Dialog, IconButton, Input, Popover, PopoverContent, PopoverTrigger } from "@/shared/ui";
import { useAddCalendar, useDeleteCalendar, useRefreshCalendar } from "../api";
import { Dot, type Calendar } from "./CalendarParts";

// The palette lives in tokens.css (--color-cal-N); the server stores the hex value.
const swatches = [1, 2, 3, 4, 5, 6].map((n) => `--color-cal-${n}`);
const hexOf = (token: string) => getComputedStyle(document.documentElement).getPropertyValue(token).trim();

function AddForm() {
  const { t } = useTranslation("planner");
  const add = useAddCalendar();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [colour, setColour] = useState(swatches[1]!);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !url.trim()) return;
    add.mutate(
      { name: name.trim(), url: url.trim(), ...(hexOf(colour) ? { colour: hexOf(colour) } : {}) },
      {
        onSuccess: () => {
          setName("");
          setUrl("");
        },
      },
    );
  };
  return (
    <form onSubmit={submit} className="flex flex-col gap-2 border-t border-border pt-3">
      <p className="font-semibold">{t("calendar.add")}</p>
      <Input aria-label={t("calendar.name")} placeholder={t("calendar.name")} value={name} onChange={(e) => setName(e.target.value)} maxLength={80} />
      <Input aria-label={t("calendar.url")} placeholder={t("calendar.url")} value={url} onChange={(e) => setUrl(e.target.value)} autoComplete="off" spellCheck={false} />
      <p className="text-meta text-fg-subtle">{t("calendar.urlHint")}</p>
      <div role="radiogroup" aria-label={t("calendar.colour")} className="flex gap-1.5">
        {swatches.map((c, i) => (
          <button
            key={c}
            type="button"
            role="radio"
            aria-checked={colour === c}
            aria-label={`${t("calendar.colour")} ${i + 1}`}
            onClick={() => setColour(c)}
            className="size-5 rounded-full ring-offset-2 ring-offset-surface-raised aria-checked:ring-2 aria-checked:ring-fg"
            style={{ background: `var(${c})` }}
          />
        ))}
      </div>
      <Button type="submit" size="sm" disabled={add.isPending || !name.trim() || !url.trim()} className="self-end">
        {t("calendar.save")}
      </Button>
    </form>
  );
}

function Row({ cal, hidden, onToggle }: { cal: Calendar; hidden: boolean; onToggle: () => void }) {
  const { t, i18n } = useTranslation("planner");
  const locale = localeOf(i18n.language);
  const refresh = useRefreshCalendar();
  const del = useDeleteCalendar();
  const [confirm, setConfirm] = useState(false);
  return (
    <li className="flex items-start gap-2 py-1.5">
      <Checkbox checked={!hidden} onCheckedChange={onToggle} aria-label={t("calendar.show", { name: cal.name })} className="mt-0.5" />
      <div className="min-w-0 flex-1">
        <p className="flex items-center gap-1.5 font-medium">
          <Dot colour={cal.colour} />
          <span className="truncate">{cal.name}</span>
        </p>
        <p className="truncate font-mono text-meta text-fg-subtle">{cal.url_masked}</p>
        <p className={cal.last_error ? "text-meta text-danger" : "text-meta text-fg-subtle"}>
          {cal.last_error
            ? t("calendar.error", { error: cal.last_error })
            : cal.last_fetched_at
              ? t("calendar.fetched", { when: relativeTime(cal.last_fetched_at, new Date(), locale) })
              : t("calendar.neverFetched")}
        </p>
      </div>
      <IconButton size="sm" label={t("calendar.refresh")} icon={<RefreshCw className={refresh.isPending ? "size-3.5 animate-spin" : "size-3.5"} />} onClick={() => refresh.mutate(cal.id)} />
      <IconButton size="sm" label={t("calendar.remove")} icon={<Trash2 className="size-3.5" />} onClick={() => setConfirm(true)} />
      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("calendar.removeTitle")}
        description={t("calendar.removeBody", { name: cal.name })}
        closeLabel={t("calendar.close")}
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(false)}>
              {t("calendar.cancel")}
            </Button>
            <Button variant="danger" onClick={() => del.mutate(cal.id, { onSuccess: () => setConfirm(false) })}>
              {t("calendar.remove")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </li>
  );
}

/** The calendars: visibility per subscription, plus adding, refreshing and removing them (round 28). */
export function CalendarsPopover({ calendars, hidden, onToggle }: { calendars: Calendar[]; hidden: Set<string>; onToggle: (id: string) => void }) {
  const { t } = useTranslation("planner");
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="secondary" size="sm">
          <CalendarDays aria-hidden className="size-4" />
          {t("calendar.calendars")}
        </Button>
      </PopoverTrigger>
      <PopoverContent label={t("calendar.calendars")} align="end" className="w-96">
        <ul className="divide-y divide-border">
          {calendars.map((c) => (
            <Row key={c.id} cal={c} hidden={hidden.has(c.id)} onToggle={() => onToggle(c.id)} />
          ))}
        </ul>
        <AddForm />
      </PopoverContent>
    </Popover>
  );
}
