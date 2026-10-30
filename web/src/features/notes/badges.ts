/** The notes circle's badge: a dot for new notes in the current thread (filled in later). */
export function useNotesBadge(): { text: string | null; news: boolean } {
  return { text: null, news: false };
}
