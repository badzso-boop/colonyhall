// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The emoji picker and the composer's ":" autocomplete (round 55, part C).
// The data (data/emoji.json) is only in the lazy chunks.
export { EmojiButton } from "./components/EmojiButton";
export { useEmojiSuggest } from "./components/useEmojiSuggest";
export { useRecordEmoji, emojiKeys } from "./api";
export { activeColon, emojiName, insertAtColon, type Emoji } from "./model";
