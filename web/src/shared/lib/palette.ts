// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

// The fixed palette for machines and agents (round 33; tokens.css
// --color-hue-<name>-solid|soft|fg). The server stores the name, never hex.

export const hues = ["red", "orange", "amber", "lime", "green", "teal", "cyan", "blue", "violet", "pink"] as const;
export type Hue = (typeof hues)[number];

export function isHue(v: unknown): v is Hue {
  return typeof v === "string" && (hues as readonly string[]).includes(v);
}

/** A CSS value for one of the hue's tokens; undefined for no or an unknown colour. */
export function hueVar(hue: string | null | undefined, part: "solid" | "soft" | "fg" = "solid"): string | undefined {
  return isHue(hue) ? `var(--color-hue-${hue}-${part})` : undefined;
}
