// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useTranslation } from "react-i18next";
import { Hammer } from "lucide-react";
import { useSession } from "@/features/identity";
import { useLocalStorage } from "@/shared/hooks";
import {
  EmptyState,
  PageHeader,
  Tab,
  TabList,
  TabPanel,
  Tabs,
} from "@/shared/ui";
import { CallsTab } from "../components/CallsTab";
import { ConnectorsTab } from "../components/ConnectorsTab";
import { GrantsTab } from "../components/GrantsTab";

/** Műhely (/workshop, round 56): connectors, grants and the call log; owner and admins. */
export default function WorkshopPage() {
  const { t } = useTranslation("workshop");
  const session = useSession();
  const role = session.data?.actor.role;
  const [tab, setTab] = useLocalStorage("workshop.tab", "connectors");
  const icon = <Hammer className="size-5" />;
  if (!session.isSuccess) return <div className="flex-1" />;
  if (role !== "owner" && role !== "admin") {
    return (
      <div className="flex h-full min-h-0 flex-col">
        <PageHeader icon={icon} title={t("title")} />
        <EmptyState icon={icon} title={t("title")} body={t("notFound")} />
      </div>
    );
  }
  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader icon={icon} title={t("title")} />
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        <div className="mx-auto flex max-w-4xl flex-col gap-4">
          <p className="text-ui text-fg-muted">{t("intro")}</p>
          <Tabs value={tab} onValueChange={setTab}>
            <TabList label={t("title")}>
              <Tab value="connectors">{t("tabs.connectors")}</Tab>
              <Tab value="grants">{t("tabs.grants")}</Tab>
              <Tab value="calls">{t("tabs.calls")}</Tab>
            </TabList>
            <TabPanel value="connectors" className="pt-4">
              <ConnectorsTab owner={role === "owner"} />
            </TabPanel>
            <TabPanel value="grants" className="pt-4">
              <GrantsTab />
            </TabPanel>
            <TabPanel value="calls" className="pt-4">
              <CallsTab />
            </TabPanel>
          </Tabs>
        </div>
      </div>
    </div>
  );
}
