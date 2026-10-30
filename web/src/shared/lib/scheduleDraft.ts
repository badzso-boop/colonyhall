import { create } from "zustand";

/**
 * A request to open the schedule builder (round 32, part 6), from the
 * composer's "+" menu or the schedules page. The builder lives in the
 * schedules feature; this store keeps the composer from importing it.
 */
export type ScheduleDraft = {
  body: string;
  channelId?: string;
  threadRootId?: string;
  cron?: string;
  timezone?: string;
  catchUp?: "once" | "skip";
  /** Called after the schedule is saved (the composer clears its text). */
  onSaved?: () => void;
};

export const useScheduleDraft = create<{ draft: ScheduleDraft | null }>(() => ({ draft: null }));

export function openScheduleBuilder(draft: ScheduleDraft): void {
  useScheduleDraft.setState({ draft });
}

export function closeScheduleBuilder(): void {
  useScheduleDraft.setState({ draft: null });
}
