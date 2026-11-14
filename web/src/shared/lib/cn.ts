// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { extendTailwindMerge } from "tailwind-merge";

// Our type scale (tokens.css: text-meta … text-display) must be known as
// font sizes, or text-ui and text-fg-muted would look like two colours and
// one of them would be dropped.
const twMerge = extendTailwindMerge({
  extend: { theme: { text: ["meta", "ui", "body", "title", "page", "display"] } },
});

// cn joins class names, skipping false, null and undefined, and lets a later
// Tailwind class win over an earlier conflicting one, so a component's
// defaults can be overridden: cn("w-full h-8", "w-36") → "h-8 w-36".
export function cn(...parts: Array<string | false | null | undefined>): string {
  return twMerge(parts.filter(Boolean).join(" "));
}
