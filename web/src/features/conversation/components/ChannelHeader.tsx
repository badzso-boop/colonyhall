import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Archive, AtSign, Bell, BellOff, BellRing, Check, Hash, Lock, LockKeyhole, LockKeyholeOpen, Settings } from "lucide-react";
import { useNow } from "@/shared/hooks";
import { cn, fullTime, hueVar } from "@/shared/lib";
import { Avatar, Button, IconButton, Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, toast } from "@/shared/ui";
import { useMembers, useMute, useNotifyLevel, useSetArchived, useSetLocked } from "../api";
import { isMuted, muteChoices, muteUntil, notifyLevel, notifyLevels, type ChannelTab, type NotifyLevel } from "../channel";
import type { Actor, Channel } from "../model";
import { PinsButton } from "./PinsButton";

const URL_PATTERN = /(https?:\/\/[^\s<>()]+)/g;

/** A description as plain text with clickable links (never markup). */
export function LinkedText({ text }: { text: string }) {
  const parts = text.split(URL_PATTERN);
  return (
    <>
      {parts.map((p, i) =>
        i % 2 === 1 ? (
          <a key={i} href={p} target="_blank" rel="noreferrer noopener" className="text-accent-text underline">
            {p}
          </a>
        ) : (
          p
        ),
      )}
    </>
  );
}

const levelIcon: Record<NotifyLevel, typeof Bell> = { all: BellRing, mentions: AtSign, none: BellOff };

/**
 * The notification level (round 55, part I): all messages, only mentions,
 * or nothing (a mute until unmuted), the current one ticked; then the
 * timed mutes of round 34.
 */
export function NotifyMenuItems({ channel }: { channel: Channel }) {
  const { t } = useTranslation("conversation");
  const set = useNotifyLevel(channel.id);
  const now = useNow(60_000);
  const current = notifyLevel(channel, now);
  return (
    <>
      <p className="px-2 pt-1 pb-0.5 text-meta font-semibold text-fg-subtle">{t("notify.title")}</p>
      {notifyLevels.map((l) => {
        const Icon = levelIcon[l];
        return (
          <MenuItem
            key={l}
            icon={<Icon className="size-4" />}
            onSelect={() => set.mutate(l, { onError: (e) => toast(e.message, { tone: "danger" }) })}
          >
            <span className="flex-1">{t(`notify.${l}`)}</span>
            {l === current ? <Check aria-label={t("notify.current")} className="ml-3 size-4 text-accent-text" /> : null}
          </MenuItem>
        );
      })}
      <MenuSeparator />
      <MuteMenuItems channel={channel} timedOnly />
    </>
  );
}

/** "Némítás" with its durations, or "Némítás feloldása" while muted. */
export function MuteMenuItems({ channel, timedOnly }: { channel: Channel; timedOnly?: boolean }) {
  const { t, i18n } = useTranslation("conversation");
  const mute = useMute(channel.id);
  const now = useNow(60_000);
  const run = (until: string | null | "off") => mute.mutate(until, { onError: (e) => toast(e.message, { tone: "danger" }) });
  if (isMuted(channel, now))
    return (
      <MenuItem icon={<Bell className="size-4" />} onSelect={() => run("off")}>
        {t("mute.off")}
        {channel.muted_until ? <span className="ml-auto text-meta text-fg-subtle">{fullTime(channel.muted_until, i18n.language)}</span> : null}
      </MenuItem>
    );
  return (
    <>
      {muteChoices.filter((c) => !timedOnly || c !== "forever").map((c) => (
        <MenuItem key={c} icon={c === "1h" ? <BellOff className="size-4" /> : <span className="size-4" />} onSelect={() => run(muteUntil(c, new Date()))}>
          {t(`mute.${c}`)}
        </MenuItem>
      ))}
    </>
  );
}

/**
 * The conversation header (round 34, 1.3): the name, the description
 * on one line ("több" expands it), the member stack that opens the members,
 * the mute state and ⚙ for admins.
 */
export function ChannelHeader({ channel, title, actors, admin, onSettings, extra }: {
  channel: Channel;
  title: string;
  actors: Map<string, Actor>;
  admin: boolean;
  onSettings: (tab: ChannelTab) => void;
  extra?: ReactNode;
}) {
  const { t, i18n } = useTranslation("conversation");
  const [open, setOpen] = useState(false);
  const now = useNow(60_000);
  const members = useMembers(channel.id);
  const mute = useMute(channel.id);
  const muted = isMuted(channel, now);
  const setLocked = useSetLocked(channel.id);
  const locked = Boolean(channel.agent_locked_at);
  const lock = (on: boolean) =>
    setLocked.mutate(on, {
      onSuccess: () => toast(t(on ? "channel.lock.done" : "channel.lock.lifted")),
      onError: (e) => toast(e.message, { tone: "danger" }),
    });
  const isChannel = channel.kind !== "dm";
  const people = (members.data ?? []).map((m) => actors.get(m.actor_id)).filter((a): a is Actor => a !== undefined);
  const shown = [...people.filter((a) => a.kind === "human"), ...people.filter((a) => a.kind === "agent")].slice(0, 4);
  return (
    <header className="shrink-0 border-b border-border bg-surface">
      <div className="flex h-12 items-center gap-2 px-4">
        {channel.kind === "private" ? <Lock aria-hidden className="size-4 shrink-0 text-fg-subtle" /> : null}
        {channel.kind === "public" ? <Hash aria-hidden className="size-4 shrink-0 text-fg-subtle" /> : null}
        <h1 className="text-title font-semibold whitespace-nowrap">{title}</h1>
        {channel.archived_at ? <Archive aria-label={t("channel.archivedBar")} className="size-4 shrink-0 text-fg-subtle" /> : null}
        {muted ? <BellOff aria-label={t("mute.muted")} className="size-4 shrink-0 text-fg-subtle" /> : null}
        {locked ? <LockKeyhole aria-label={t("channel.lock.locked")} className="size-4 shrink-0 text-warning" /> : null}
        {channel.description && !open ? (
          <p className="min-w-0 flex-1 truncate text-ui text-fg-muted">
            <LinkedText text={channel.description} />
          </p>
        ) : (
          <span className="flex-1" />
        )}
        {channel.description && channel.description.length > 60 ? (
          <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="shrink-0 text-meta text-accent-text hover:underline">
            {t(open ? "message.less" : "message.more")}
          </button>
        ) : null}
        {extra}
        <PinsButton channelId={channel.id} actors={actors} />
        {isChannel && members.data ? (
          <button
            type="button"
            onClick={() => onSettings("tagok")}
            aria-label={t("channel.members.open", { count: members.data.length })}
            className="flex shrink-0 items-center rounded-md px-1 py-0.5 hover:bg-surface-sunken"
          >
            {shown.map((a, i) => (
              <Avatar
                key={a.id}
                kind={a.kind === "agent" ? "agent" : "human"}
                name={a.display_name || a.handle}
                emoji={a.avatar_emoji}
                size="sm"
                ringColor={hueVar(a.effective_color)}
                className={cn(i > 0 && "-ml-1.5", "ring-2 ring-surface", locked && a.kind === "agent" && "opacity-50")}
              />
            ))}
            <span className="ml-1 text-meta text-fg-muted tabular-nums">{members.data.length}</span>
          </button>
        ) : null}
        <Menu>
          <MenuTrigger asChild>
            <IconButton size="sm" label={t("mute.menu")} icon={muted ? <BellOff className="size-4" /> : <Bell className="size-4" />} />
          </MenuTrigger>
          <MenuContent>
            <NotifyMenuItems channel={channel} />
            {isChannel ? (
              <>
                <MenuSeparator />
                <MenuItem icon={<Settings className="size-4" />} onSelect={() => onSettings("altalanos")}>
                  {t(admin ? "channel.settings" : "channel.info")}
                </MenuItem>
                {admin && !channel.archived_at ? (
                  <MenuItem icon={locked ? <LockKeyholeOpen className="size-4" /> : <LockKeyhole className="size-4" />} onSelect={() => lock(!locked)}>
                    {t(locked ? "channel.lock.unlock" : "channel.lock.lock")}
                  </MenuItem>
                ) : null}
              </>
            ) : null}
          </MenuContent>
        </Menu>
        {isChannel && admin ? <IconButton size="sm" label={t("channel.settings")} icon={<Settings className="size-4" />} onClick={() => onSettings("altalanos")} /> : null}
      </div>
      {open && channel.description ? (
        <p className="px-4 pb-2 text-ui whitespace-pre-wrap text-fg-muted">
          <LinkedText text={channel.description} />
        </p>
      ) : null}
      {locked ? (
        <div role="status" className="flex items-center gap-2 border-t border-border bg-warning-soft px-4 py-1 text-meta text-warning">
          <LockKeyhole aria-hidden className="size-3.5 shrink-0" />
          <span className="flex-1">{t("channel.lock.bar")}</span>
          {admin ? (
            <Button size="sm" variant="ghost" loading={setLocked.isPending} onClick={() => lock(false)}>
              {t("channel.lock.unlock")}
            </Button>
          ) : null}
        </div>
      ) : null}
      {muted ? (
        <div className="flex items-center gap-2 border-t border-border px-4 py-1 text-meta text-fg-muted">
          <BellOff aria-hidden className="size-3.5" />
          <span className="flex-1">{channel.muted_until ? t("mute.until", { when: fullTime(channel.muted_until, i18n.language) }) : t("mute.forever_state")}</span>
          <Button size="sm" variant="ghost" loading={mute.isPending} onClick={() => mute.mutate("off")}>
            {t("mute.off")}
          </Button>
        </div>
      ) : null}
    </header>
  );
}

/** Replaces the composer in an archived channel; admins can restore it. */
export function ArchivedBar({ channel, admin }: { channel: Channel; admin: boolean }) {
  const { t } = useTranslation("conversation");
  const restore = useSetArchived(channel.id);
  return (
    <div role="status" className="flex shrink-0 items-center gap-2 border-t border-border bg-surface-sunken px-4 py-3 text-ui text-fg-muted">
      <Archive aria-hidden className="size-4" />
      <span className="flex-1">{t("channel.archivedBar")}</span>
      {admin ? (
        <Button size="sm" loading={restore.isPending} onClick={() => restore.mutate(false, { onError: (e) => toast(e.message, { tone: "danger" }) })}>
          {t("channel.archive.restore")}
        </Button>
      ) : null}
    </div>
  );
}
