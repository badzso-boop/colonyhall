// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Building2, Plus } from "lucide-react";
import { fullTime } from "@/shared/lib";
import { Button, EmptyState, PageHeader, Skeleton } from "@/shared/ui";
import { useCustomers } from "../api";
import { CreateCustomerDialog } from "../components/CreateCustomerDialog";
import { OwnerBadge, StateBadge, StorageText } from "../components/CustomerBadges";

/** Ügyfelek: every workspace with its account data and numbers, no content. */
export default function CustomersPage() {
  const { t, i18n } = useTranslation("operator");
  const customers = useCustomers();
  const [creating, setCreating] = useState(false);
  const now = new Date();
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        icon={<Building2 className="size-5" />}
        title={t("customers.title")}
        actions={
          <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => setCreating(true)}>
            {t("customers.new")}
          </Button>
        }
      />
      {customers.isPending ? (
        <Skeleton className="m-4 h-32" />
      ) : customers.isError ? (
        <p className="p-4 text-danger">{customers.error.message}</p>
      ) : !customers.data.length ? (
        <EmptyState icon={<Building2 className="size-6" />} title={t("customers.empty")} />
      ) : (
        <div className="min-h-0 flex-1 overflow-auto p-3 sm:p-4">
          <table className="w-full min-w-[56rem] text-ui">
            <caption className="sr-only">{t("customers.title")}</caption>
            <thead className="border-b border-border text-left text-meta text-fg-subtle">
              <tr>
                <th scope="col" className="px-2 py-1.5 font-medium">{t("customers.name")}</th>
                <th scope="col" className="px-2 py-1.5 font-medium">{t("customers.state")}</th>
                <th scope="col" className="px-2 py-1.5 font-medium">{t("customers.owner")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("customers.users")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("customers.agents")}</th>
                <th scope="col" className="px-2 py-1.5 text-right font-medium">{t("customers.hosts")}</th>
                <th scope="col" className="px-2 py-1.5 font-medium">{t("customers.storage")}</th>
                <th scope="col" className="px-2 py-1.5 font-medium">{t("customers.lastActivity")}</th>
              </tr>
            </thead>
            <tbody>
              {customers.data.map((c) => (
                <tr key={c.id} className="border-b border-border/60 last:border-0">
                  <td className="px-2 py-2">
                    <Link to={`/customers/${c.id}`} className="font-medium text-accent-text hover:underline">
                      {c.name}
                    </Link>
                    <span className="block font-mono text-meta text-fg-muted">{c.slug}</span>
                  </td>
                  <td className="px-2">
                    <StateBadge state={c.state} />
                  </td>
                  <td className="px-2">
                    <OwnerBadge customer={c} now={now} />
                  </td>
                  <td className="tabular px-2 text-right">{c.users}</td>
                  <td className="tabular px-2 text-right">{c.agents}</td>
                  <td className="tabular px-2 text-right">{t("customers.hostsValue", { online: c.hosts_online, total: c.hosts })}</td>
                  <td className="px-2">
                    <StorageText storage={c.storage} />
                  </td>
                  <td className="px-2 whitespace-nowrap text-fg-muted">{c.last_activity_at ? fullTime(c.last_activity_at, i18n.language) : t("never")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {creating ? <CreateCustomerDialog onClose={() => setCreating(false)} /> : null}
    </div>
  );
}
