// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useConnection } from "@/shared/api";

const GRACE_MS = 3000;

/** "Újracsatlakozás…" under the header, only after 3 s (not on every blip). */
export function ConnectionBar() {
  const { t } = useTranslation("layout");
  const { state, since } = useConnection();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (state !== "reconnecting") return;
    const timer = setTimeout(() => setNow(Date.now()), Math.max(0, GRACE_MS - (Date.now() - since)));
    return () => clearTimeout(timer);
  }, [state, since]);
  if (state !== "reconnecting" || now - since < GRACE_MS) return null;
  return (
    <div role="status" className="bg-warning-soft px-4 py-1 text-center text-meta font-medium text-warning">
      {t("reconnecting")}
    </div>
  );
}
