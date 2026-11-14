import { unpack, type Emoji, type RawEmoji } from "./model";

let cache: Promise<Emoji[]> | null = null;

/** The emoji list, loaded once, in a chunk of its own (the bundle stays small until it is needed). */
export function loadEmoji(): Promise<Emoji[]> {
  cache ??= import("./data/emoji.json").then((m) => unpack((m.default as unknown as { emoji: RawEmoji[] }).emoji));
  return cache;
}
