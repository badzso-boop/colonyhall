// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { closeScheduleBuilder, useScheduleDraft } from "@/shared/lib";
import { ScheduleDialog } from "./ScheduleDialog";

/** Renders the builder whenever someone asked for it (openScheduleBuilder). */
export function ScheduleBuilderHost() {
  const draft = useScheduleDraft((s) => s.draft);
  return draft ? <ScheduleDialog draft={draft} onClose={closeScheduleBuilder} /> : null;
}
