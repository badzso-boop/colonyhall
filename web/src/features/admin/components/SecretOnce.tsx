// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Copy } from "lucide-react";
import { Button, Notice, toast } from "@/shared/ui";

/** A secret the server (or the browser) gives only once: selectable, with copy and a warning. */
export function SecretOnce({ secret }: { secret: string }) {
  const { t } = useTranslation("admin");
  const copy = () => {
    navigator.clipboard.writeText(secret).then(
      () => toast(t("copied")),
      () => toast(t("copyFailed"), { tone: "danger" }),
    );
  };
  return (
    <div className="flex flex-col gap-2">
      <Notice tone="warning">{t("secret.warning")}</Notice>
      <div className="flex items-center gap-2 rounded-md border border-border bg-surface-sunken p-2">
        <code className="min-w-0 flex-1 font-mono text-meta break-all select-all">{secret}</code>
        <Button size="sm" icon={<Copy className="size-4" />} onClick={copy}>
          {t("copy")}
        </Button>
      </div>
    </div>
  );
}
