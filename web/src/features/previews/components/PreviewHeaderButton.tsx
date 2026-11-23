// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { MonitorPlay } from "lucide-react";
import { insertIntoComposer } from "@/shared/lib";
import { Button } from "@/shared/ui";
import { usePreviews } from "../api";
import { previewFor } from "../model";
import { usePreviewParam } from "../pane";

/**
 * The conversation header's preview button (round 57). With a live
 * preview announced here it opens the split view; without one it puts a
 * request to the agent in the composer: the agent starts a preview with
 * its own compose file (people do not start containers on a machine).
 */
export function PreviewHeaderButton({ channelId, threadRootId, target = "main" }: { channelId: string; threadRootId?: string | null; target?: "main" | "thread" }) {
  const { t } = useTranslation("previews");
  const previews = usePreviews();
  const [openId, open] = usePreviewParam();
  const p = previewFor(previews.data ?? [], channelId, threadRootId);
  if (p) {
    return (
      <Button size="sm" variant={openId === p.id ? "primary" : "secondary"} icon={<MonitorPlay className="size-4" />} onClick={() => open(p.id)}>
        {t("open")}
      </Button>
    );
  }
  return (
    <Button size="sm" icon={<MonitorPlay className="size-4" />} title={t("startHint")} onClick={() => insertIntoComposer(t("askAgent"), target)}>
      {t("start")}
    </Button>
  );
}
