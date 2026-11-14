// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { snippetParts } from "../model";

/** A hit's snippet with the keyword matches (between « and ») marked; built from text nodes, never as HTML. */
export function Snippet({ text, className }: { text: string; className?: string }) {
  return (
    <p className={className}>
      {snippetParts(text).map((p, i) =>
        p.mark ? (
          <mark key={i} className="rounded-sm bg-warning-soft text-fg">
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </p>
  );
}
