// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { RotateCw } from "lucide-react";
import { Button, Sheet } from "@/shared/ui";
import { useMessageFile } from "../api";
import { FetchState, FileContent } from "./FileViewer";

/**
 * A local file linked in an agent's message (round 16), for humans: the
 * request goes to the author's guard (pending state while it works), then
 * the file opens read-only: Monaco for text, the safe markdown, an image
 * preview, or the head or tail of a large file. Failures are EmptyStates.
 */
export function MessageFileSheet({ messageId, path, handle, lines, onClose }: { messageId: string; path: string | null; handle: string; lines?: { start?: number | undefined; end?: number | undefined }; onClose: () => void }) {
  const { t } = useTranslation("files");
  const [nonce, setNonce] = useState(0);
  const fetched = useMessageFile(messageId, path, nonce);
  const name = path?.split("/").pop() ?? "";
  return (
    <Sheet open={path !== null} onOpenChange={(o) => !o && onClose()} title={name} description={path ?? undefined} closeLabel={t("common:close")}>
      <div className="-m-4 flex h-[calc(100dvh-5rem)] flex-col sm:h-[calc(100dvh-4.5rem)]">
        <div className="flex justify-end border-b border-border px-3 py-1">
          <Button size="sm" variant="ghost" icon={<RotateCw className="size-3.5" />} disabled={fetched.isFetching} onClick={() => setNonce((n) => n + 1)}>
            {t("refresh")}
          </Button>
        </div>
        <div className="min-h-0 flex-1">
          {path ? <FetchState fetched={fetched}>{(file) => <FileContent file={file} handle={handle} path={path} start={lines?.start} end={lines?.end} />}</FetchState> : null}
        </div>
      </div>
    </Sheet>
  );
}
