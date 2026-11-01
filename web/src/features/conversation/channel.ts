import type { Channel, Actor } from "./model";

/** The channel settings tabs (docs/handoff/34, 1.3), the URL's ?channel-settings=<tab>. */
export const channelTabs = ["altalanos", "tagok", "tomorites", "archivalas"] as const;
export type ChannelTab = (typeof channelTabs)[number];

export function isChannelTab(v: string | null): v is ChannelTab {
  return channelTabs.includes(v as ChannelTab);
}

/** The server's channel name rule: 2-32 of a-z, 0-9 and "-", not starting with "-" or "dm-". */
export function validChannelName(name: string): boolean {
  return /^[a-z0-9][a-z0-9-]{1,31}$/.test(name) && !name.startsWith("dm-");
}

export const MAX_DESCRIPTION = 250;

export const muteChoices = ["1h", "8h", "tomorrow", "week", "forever"] as const;
export type MuteChoice = (typeof muteChoices)[number];

/** When a mute choice ends (ISO), or null for "végleg". "Holnap reggelig" is tomorrow 08:00 local time. */
export function muteUntil(choice: MuteChoice, now: Date): string | null {
  const hour = 3600_000;
  switch (choice) {
    case "1h":
      return new Date(now.getTime() + hour).toISOString();
    case "8h":
      return new Date(now.getTime() + 8 * hour).toISOString();
    case "tomorrow": {
      const d = new Date(now);
      d.setDate(d.getDate() + 1);
      d.setHours(8, 0, 0, 0);
      return d.toISOString();
    }
    case "week":
      return new Date(now.getTime() + 7 * 24 * hour).toISOString();
    case "forever":
      return null;
  }
}

/** Whether the viewer's mute is in force (an expired one counts as unmuted, like on the server). */
export function isMuted(ch: Channel, now: Date): boolean {
  if (!ch.muted) return false;
  return !ch.muted_until || new Date(ch.muted_until) > now;
}

/** The members tab order: humans first, then agents grouped by their colour, each by name. */
export function orderMembers<T extends { actor: Actor }>(rows: T[]): T[] {
  const name = (a: Actor) => (a.display_name || a.handle).toLocaleLowerCase("hu");
  return [...rows].sort((x, y) => {
    const kx = x.actor.kind === "agent" ? 1 : 0;
    const ky = y.actor.kind === "agent" ? 1 : 0;
    if (kx !== ky) return kx - ky;
    if (kx === 1) {
      const c = (x.actor.effective_color ?? "~").localeCompare(y.actor.effective_color ?? "~");
      if (c !== 0) return c;
    }
    return name(x.actor).localeCompare(name(y.actor), "hu");
  });
}
