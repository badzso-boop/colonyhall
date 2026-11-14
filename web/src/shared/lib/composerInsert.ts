// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { create } from "zustand";

/**
 * Text another part of the app wants in the composer (e.g. a line reference
 * from the file browser, round 20). The composer of the focused conversation
 * or thread takes it; `nonce` makes repeated inserts of the same text count.
 */
export const useComposerInsert = create<{ text: string; nonce: number; target: "main" | "thread" }>(() => ({ text: "", nonce: 0, target: "main" }));

export function insertIntoComposer(text: string, target: "main" | "thread" = "main"): void {
  useComposerInsert.setState((s) => ({ text, nonce: s.nonce + 1, target }));
}
