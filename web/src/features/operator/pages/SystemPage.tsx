// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Database } from "lucide-react";
import { DatabaseSizeCard } from "@/features/admin";
import { PageHeader, Skeleton } from "@/shared/ui";
import { useOperatorDatabase } from "../api";

/** Rendszer: the whole database (round 50's card); in multi-tenant mode only the operator sees it. */
export default function SystemPage() {
  const { t } = useTranslation("operator");
  const db = useOperatorDatabase();
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader icon={<Database className="size-5" />} title={t("system.title")} />
      <div className="grid min-h-0 flex-1 content-start gap-3 overflow-auto p-3 sm:p-4 lg:grid-cols-2">
        {db.isPending ? <Skeleton className="h-40" /> : db.isError ? <p className="text-danger">{db.error.message}</p> : <DatabaseSizeCard database={db.data} />}
      </div>
    </div>
  );
}
