// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type Schemas } from "@/shared/api";

export type EmojiUsage = Schemas["EmojiUsage"];
export const emojiKeys = { usage: ["emoji", "usage"] as const };

/** The person's recent and frequent emoji (kept on the server, so every device shows the same). */
export function useEmojiUsage(enabled: boolean) {
  return useQuery({
    queryKey: emojiKeys.usage,
    enabled,
    staleTime: 60_000,
    queryFn: async () => unwrap(await api.GET("/me/emoji")),
  });
}

/** Records a picked emoji; it moves to the front of "recent" at once. */
export function useRecordEmoji() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (emoji: string) => {
      unwrap(await api.POST("/me/emoji", { body: { emoji } }));
    },
    onMutate: (emoji) => {
      qc.setQueryData<EmojiUsage>(emojiKeys.usage, (u) => (u ? { ...u, recent: [emoji, ...u.recent.filter((e) => e !== emoji)].slice(0, 24) } : u));
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: emojiKeys.usage }),
  });
}
