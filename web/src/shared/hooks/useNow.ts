import { useEffect, useState } from "react";

/** The current time, re-read every `everyMs` (the calendar's now line, countdowns). */
export function useNow(everyMs = 60_000): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), everyMs);
    return () => window.clearInterval(id);
  }, [everyMs]);
  return now;
}
