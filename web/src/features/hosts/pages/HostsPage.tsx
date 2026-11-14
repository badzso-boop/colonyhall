// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";
import { Plus, Server } from "lucide-react";
import { useSession } from "@/features/identity";
import { useNow, useWidth } from "@/shared/hooks";
import { Button, EmptyState, PageHeader, Skeleton } from "@/shared/ui";
import { useHosts } from "../api";
import { HostCard } from "../components/HostCard";
import { HostDetail } from "../components/HostDetail";
import { NewHostWizard } from "../components/NewHostWizard";

/** Gépek (/hosts, /hosts/:hostId): the machine list and one machine's detail; stacked on a phone. */
export default function HostsPage() {
  const { t } = useTranslation("hosts");
  const { hostId } = useParams();
  const session = useSession();
  const role = session.data?.actor.role;
  const admin = role === "owner" || role === "admin";
  // last_seen_at and the reports change without an event: re-read while the page is open.
  // A person who is not an admin gets only the machines they own (round 38).
  const hosts = useHosts(session.isSuccess, 30_000);
  const now = useNow(10_000);
  const narrow = useWidth() === "narrow";
  const [wizard, setWizard] = useState(false);

  const list = hosts.data ?? [];
  const selected = hostId ? list.find((h) => h.id === hostId) : narrow ? undefined : list[0];

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        icon={<Server className="size-5" />}
        title={t("title")}
        actions={
          admin ? (
            <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => setWizard(true)}>
              {t("new")}
            </Button>
          ) : undefined
        }
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:flex-row md:overflow-hidden">
        <nav aria-label={t("list")} className="shrink-0 border-border p-3 md:w-80 md:overflow-y-auto md:border-r">
          {hosts.isPending ? (
            <div className="flex flex-col gap-2">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : hosts.isError ? (
            <p className="text-ui text-danger">{t("loadFailed")}</p>
          ) : list.length ? (
            <ul className="flex flex-col gap-2">
              {list.map((h) => (
                <li key={h.id}>
                  <HostCard host={h} now={now} selected={h.id === selected?.id} />
                </li>
              ))}
            </ul>
          ) : (
            <EmptyState icon={<Server className="size-8" />} title={t(admin ? "empty" : "emptyOwn")} body={admin ? t("emptyBody") : t("emptyOwnBody")} />
          )}
        </nav>
        <div className="min-w-0 flex-1 border-t border-border md:overflow-y-auto md:border-t-0">
          {selected ? (
            <HostDetail key={selected.id} host={selected} now={now} admin={admin} />
          ) : hostId && hosts.isSuccess ? (
            <EmptyState title={t("notFound")} />
          ) : null}
        </div>
      </div>
      {admin ? <NewHostWizard open={wizard} onOpenChange={setWizard} /> : null}
    </div>
  );
}
