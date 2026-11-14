import { useEffect, useMemo, useState } from "react";
import { loadEmoji } from "../load";
import { activeColon, searchEmoji, type Emoji } from "../model";

/**
 * The composer's ":" autocomplete (round 55, part C): up to 8 emoji for the
 * ":word" at the caret. The data loads the first time a ":word" is typed.
 */
export function useEmojiSuggest(text: string, caret: number): Emoji[] {
  const colon = activeColon(text, caret);
  const [list, setList] = useState<Emoji[] | null>(null);
  const wanted = colon !== null;
  useEffect(() => {
    if (!wanted || list) return;
    let live = true;
    void loadEmoji().then((l) => live && setList(l));
    return () => {
      live = false;
    };
  }, [wanted, list]);
  const query = colon?.query ?? "";
  return useMemo(() => (list && query ? searchEmoji(list, query, 8) : []), [list, query]);
}
