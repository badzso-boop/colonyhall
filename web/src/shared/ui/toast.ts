// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { create } from "zustand";

export type ToastAction = { label: string; run: () => void };
export type ToastItem = { id: number; text: string; tone: "neutral" | "danger"; action?: ToastAction; until: number };

type ToastState = {
  items: ToastItem[];
  push: (t: Omit<ToastItem, "id" | "until">, ms?: number) => number;
  dismiss: (id: number) => void;
};

let nextId = 1;

/** Feedback that needs no answer (docs/07): at most 3 at a time, newest last. */
export const useToasts = create<ToastState>((set) => ({
  items: [],
  push: (t, ms = 5000) => {
    const id = nextId++;
    set((s) => ({ items: [...s.items, { ...t, id, until: Date.now() + ms }].slice(-3) }));
    setTimeout(() => set((s) => ({ items: s.items.filter((i) => i.id !== id) })), ms);
    return id;
  },
  dismiss: (id) => set((s) => ({ items: s.items.filter((i) => i.id !== id) })),
}));

/** toast("Feltöltve") or toast("Elküldve", { action: { label: "Visszavonás", run }, ms: 10000 }). */
export function toast(text: string, opts: { tone?: "neutral" | "danger"; action?: ToastAction; ms?: number } = {}): number {
  return useToasts.getState().push({ text, tone: opts.tone ?? "neutral", action: opts.action }, opts.ms);
}
