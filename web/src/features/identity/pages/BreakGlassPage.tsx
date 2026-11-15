// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { ApiError } from "@/shared/api";
import { BrandMark, Skeleton } from "@/shared/ui";
import { useRedeemBreakGlass } from "../api";
import { inviteToken } from "../model";

/**
 * The operator's break-glass link (round 51b): /app/break-glass#<token>,
 * 60 seconds, once. It opens the read-only session and goes to the app;
 * the token stays in the fragment, so it is never in a server log.
 */
export default function BreakGlassPage() {
  const { t } = useTranslation("identity");
  const navigate = useNavigate();
  const [token] = useState(() => inviteToken(globalThis.location?.hash ?? ""));
  const redeem = useRedeemBreakGlass();
  const started = useRef(false);
  useEffect(() => {
    if (!token || started.current) return;
    started.current = true;
    redeem.mutate(token, {
      onSuccess: () => {
        globalThis.history?.replaceState(null, "", "/app/");
        navigate("/", { replace: true });
      },
    });
  }, [token, redeem, navigate]);
  const gone = !token || (redeem.error instanceof ApiError && redeem.error.status === 410);
  return (
    <main className="grid min-h-full place-items-center px-4">
      <div className="flex w-full max-w-sm flex-col gap-4 rounded-lg border border-border bg-surface p-6 shadow-raised">
        <div className="flex items-center gap-2">
          <BrandMark className="size-7" />
          <span className="text-title font-semibold">{t("common:appName")}</span>
        </div>
        {redeem.isError || !token ? (
          <>
            <h1 className="text-page font-semibold">{t("breakGlass.invalidTitle")}</h1>
            <p role="alert" className="text-ui text-fg-muted">
              {gone ? t("breakGlass.gone") : redeem.error?.message}
            </p>
          </>
        ) : (
          <>
            <h1 className="text-page font-semibold">{t("breakGlass.opening")}</h1>
            <Skeleton className="h-4 w-2/3" />
          </>
        )}
      </div>
    </main>
  );
}
