import { useState } from "react";
import { useTranslation } from "react-i18next";
import { hueVar } from "@/shared/lib";
import { Tab, TabList, TabPanel, Tabs } from "@/shared/ui";
import type { Host } from "../api";
import { hostWarnings } from "../model";
import { AgentsTab } from "./AgentsTab";
import { FoldersTab } from "./FoldersTab";
import { OnlineBadge, WarningBadge } from "./HostCard";
import { OverviewTab } from "./OverviewTab";
import { SettingsTab } from "./SettingsTab";
import { WorktreesTab } from "./WorktreesTab";

const tabs = ["overview", "agents", "folders", "worktrees", "settings"] as const;
type TabId = (typeof tabs)[number];
// A machine's owner who is not an admin (round 38): folders and settings are admin-only.
const memberTabs: readonly TabId[] = ["overview", "agents", "worktrees"];

/** One machine: a header and five tabs (docs/handoff/33, part 5). */
export function HostDetail({ host, now, admin = true }: { host: Host; now: Date; admin?: boolean }) {
  const { t } = useTranslation("hosts");
  const shown = admin ? tabs : memberTabs;
  const [tab, setTab] = useState<TabId>("overview");
  return (
    <section aria-labelledby={`host-title-${host.id}`} className="flex min-w-0 flex-col gap-3 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span aria-hidden className="size-3 shrink-0 rounded-full" style={{ background: hueVar(host.color) }} />
        <h2 id={`host-title-${host.id}`} className="mr-auto text-title font-semibold">
          {host.name}
        </h2>
        <OnlineBadge host={host} now={now} />
        {hostWarnings(host).map((w) => (
          <WarningBadge key={w.kind} w={w} />
        ))}
      </div>
      <Tabs value={tab} onValueChange={(v) => setTab(v as TabId)}>
        <div className="overflow-x-auto">
          <TabList label={t("tabs.label")}>
            {shown.map((id) => (
              <Tab key={id} value={id}>
                {t(`tabs.${id}`)}
              </Tab>
            ))}
          </TabList>
        </div>
        <div className="pt-4">
          <TabPanel value="overview">
            <OverviewTab host={host} now={now} />
          </TabPanel>
          <TabPanel value="agents">
            <AgentsTab host={host} />
          </TabPanel>
          {admin ? (
            <TabPanel value="folders">
              <FoldersTab host={host} />
            </TabPanel>
          ) : null}
          <TabPanel value="worktrees">
            <WorktreesTab host={host} now={now} />
          </TabPanel>
          {admin ? (
            <TabPanel value="settings">
              <SettingsTab key={`${host.name}:${host.color}`} host={host} />
            </TabPanel>
          ) : null}
        </div>
      </Tabs>
    </section>
  );
}
