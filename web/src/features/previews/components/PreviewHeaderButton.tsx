// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { MonitorPlay, RotateCw } from "lucide-react";
import { insertIntoComposer } from "@/shared/lib";
import { Button, toast } from "@/shared/ui";
import { usePreviews, useRestartPreview } from "../api";
import { lastPreviewFor, previewFor } from "../model";
import { usePreviewParam } from "../pane";

/** The button text, hidden on phones where the header only fits the icon. */
function Label({ children }: { children: string }) {
  return <span className="max-sm:hidden">{children}</span>;
}

/**
 * The conversation header's preview button (round 57). With a live
 * preview announced here it opens the split view; with a stopped one it
 * restarts it (the agent's machine runs the same compose file again) and
 * opens it; with none it puts a request to the agent in the composer:
 * only the agent knows which compose file to start the first time.
 */
export function PreviewHeaderButton({ channelId, threadRootId, target = "main" }: { channelId: string; threadRootId?: string | null; target?: "main" | "thread" }) {
  const { t } = useTranslation("previews");
  const previews = usePreviews();
  const restart = useRestartPreview();
  const [openId, open] = usePreviewParam();
  const list = previews.data ?? [];
  const p = previewFor(list, channelId, threadRootId);
  if (p) {
    return (
      <Button size="sm" variant={openId === p.id ? "primary" : "secondary"} icon={<MonitorPlay className="size-4" />} aria-label={t("open")} onClick={() => open(p.id)}>
        <Label>{t("open")}</Label>
      </Button>
    );
  }
  const last = lastPreviewFor(list, channelId, threadRootId);
  if (last) {
    return (
      <Button
        size="sm"
        icon={<RotateCw className="size-4" />}
        title={t("restartHint", { name: last.name })}
        aria-label={t("restart")}
        loading={restart.isPending}
        onClick={() =>
          restart.mutate(last.id, {
            onSuccess: (np) => open(np.id),
            onError: () => toast(t("restartFailed"), { tone: "danger" }),
          })
        }
      >
        <Label>{t("restart")}</Label>
      </Button>
    );
  }
  return (
    <Button size="sm" icon={<MonitorPlay className="size-4" />} title={t("startHint")} aria-label={t("start")} onClick={() => insertIntoComposer(t("askAgent"), target)}>
      <Label>{t("start")}</Label>
    </Button>
  );
}
