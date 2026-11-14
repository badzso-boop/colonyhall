// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { Fragment } from "react";
import { matchRanges } from "@/shared/lib";

/** Text with the parts a search query matched marked. */
export function Highlight({ text, query }: { text: string; query: string }) {
  const ranges = matchRanges(text, query);
  if (!ranges.length) return <>{text}</>;
  const out = [];
  let at = 0;
  for (const [a, b] of ranges) {
    if (a > at) out.push(<Fragment key={`t${at}`}>{text.slice(at, a)}</Fragment>);
    out.push(
      <mark key={`m${a}`} className="rounded-sm bg-warning-soft text-fg">
        {text.slice(a, b)}
      </mark>,
    );
    at = b;
  }
  if (at < text.length) out.push(<Fragment key={`t${at}`}>{text.slice(at)}</Fragment>);
  return <>{out}</>;
}
