// The emoji picker and the composer's ":" autocomplete (round 55, part C).
// The data (data/emoji.json) is only in the lazy chunks.
export { EmojiButton } from "./components/EmojiButton";
export { useEmojiSuggest } from "./components/useEmojiSuggest";
export { useRecordEmoji, emojiKeys } from "./api";
export { activeColon, emojiName, insertAtColon, type Emoji } from "./model";
