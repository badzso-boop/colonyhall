// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { api, onResync, onUntypedEvent, unwrap, type Schemas } from "@/shared/api";

export type WikiIndex = Schemas["WikiPageIndex"];
export type WikiPage = Schemas["WikiPage"];

export const wikiKeys = {
  all: ["wiki"] as const,
  list: (q: string, deleted: boolean) => ["wiki", "list", q, deleted] as const,
  page: (slug: string) => ["wiki", "page", slug] as const,
  revisions: (slug: string) => ["wiki", "revisions", slug] as const,
};

export function useWikiIndex(q: string, deleted: boolean) {
  return useQuery({
    queryKey: wikiKeys.list(q, deleted),
    queryFn: async () => unwrap(await api.GET("/wiki", { params: { query: { ...(q ? { q } : {}), ...(deleted ? { deleted: 1 as const } : {}) } } })).pages,
    placeholderData: (prev) => prev,
  });
}

export function useWikiPage(slug: string) {
  return useQuery({ queryKey: wikiKeys.page(slug), queryFn: async () => unwrap(await api.GET("/wiki/{slug}", { params: { path: { slug } } })), retry: false });
}

export function useWikiRevisions(slug: string) {
  return useQuery({ queryKey: wikiKeys.revisions(slug), queryFn: async () => unwrap(await api.GET("/wiki/{slug}/revisions", { params: { path: { slug } } })).revisions });
}

function useWikiMutation<V>(fn: (v: V) => Promise<unknown>) {
  const qc = useQueryClient();
  return useMutation({ mutationFn: fn, onSettled: () => void qc.invalidateQueries({ queryKey: wikiKeys.all }) });
}

export const useCreateTopic = () =>
  useWikiMutation(async (body: Schemas["CreateWikiPageRequest"]) => unwrap(await api.POST("/wiki", { body })));
export const useAppend = (slug: string) =>
  useWikiMutation(async (text: string) => unwrap(await api.POST("/wiki/{slug}/append", { params: { path: { slug } }, body: { text } })));
export const useEditTopic = (slug: string) =>
  useWikiMutation(async (body: Schemas["EditWikiPageRequest"]) => unwrap(await api.PUT("/wiki/{slug}", { params: { path: { slug } }, body })));
export const useDeleteTopic = (slug: string) =>
  useWikiMutation(async () => unwrap(await api.DELETE("/wiki/{slug}", { params: { path: { slug } } })));
export const useRestoreTopic = () =>
  useWikiMutation(async (slug: string) => unwrap(await api.POST("/wiki/{slug}/restore", { params: { path: { slug } } })));

export function registerWikiEvents(qc: QueryClient): () => void {
  const refetch = () => void qc.invalidateQueries({ queryKey: wikiKeys.all });
  const offs = [...["wiki.created", "wiki.appended", "wiki.edited", "wiki.deleted", "wiki.restored"].map((t) => onUntypedEvent(t, refetch)), onResync(refetch)];
  return () => offs.forEach((off) => off());
}

// The "Új" marker: when this viewer last opened each topic (this browser only).
const SeenKey = "colony.wiki.seen";

export function seenTopics(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(SeenKey) ?? "{}") as Record<string, string>;
  } catch {
    return {};
  }
}

export function markTopicSeen(slug: string, at: string): void {
  try {
    localStorage.setItem(SeenKey, JSON.stringify({ ...seenTopics(), [slug]: at }));
  } catch {
    // Private mode: the marker simply stays.
  }
}
