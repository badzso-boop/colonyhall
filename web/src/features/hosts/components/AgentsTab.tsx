import { useTranslation } from "react-i18next";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Bot, PanelRight, Plus, Settings } from "lucide-react";
import { useActors } from "@/features/identity";
import { usePanes } from "@/shared/panes";
import { Badge, Button, EmptyState } from "@/shared/ui";
import type { Host } from "../api";
import { AgentAvatar } from "./bits";

/** Agentek: the machine's agents in a compact table, linking to the overview pane and the settings dialog. */
export function AgentsTab({ host }: { host: Host }) {
  const { t } = useTranslation("hosts");
  const actors = useActors();
  const [, dispatch] = usePanes();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const settingsSearch = (handle: string) => {
    const next = new URLSearchParams(params);
    next.set("agent", handle);
    next.set("settings", "profil");
    return `?${next.toString()}`;
  };
  const newAgent = (
    <Button variant="primary" size="sm" icon={<Plus className="size-4" />} onClick={() => navigate(`/agents?new=1&host=${host.id}`)}>
      {t("agents.new")}
    </Button>
  );
  const agents = [...host.agents].sort((a, b) => a.handle.localeCompare(b.handle));

  if (!agents.length) return <EmptyState icon={<Bot className="size-8" />} title={t("agents.empty")} action={newAgent} />;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex justify-end">{newAgent}</div>
      <div className="overflow-x-auto">
        <table className="w-full text-ui">
          <thead className="text-left text-meta text-fg-muted">
            <tr className="border-b border-border">
              <th className="py-1.5 pr-3 font-medium">{t("agents.agent")}</th>
              <th className="py-1.5 pr-3 font-medium">{t("agents.client")}</th>
              <th className="py-1.5 pr-3 font-medium">{t("agents.access")}</th>
              <th className="py-1.5 pr-3 font-medium">{t("agents.grants")}</th>
              <th className="py-1.5 font-medium">
                <span className="sr-only">{t("agents.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {agents.map((a) => {
              const actor = actors.data?.get(a.agent_id);
              return (
                <tr key={a.agent_id} className="border-b border-border">
                  <td className="py-1.5 pr-3">
                    <span className="flex items-center gap-2">
                      <AgentAvatar actor={actor} handle={a.handle} hostColor={host.color} size="sm" />
                      <span className="min-w-0">
                        <span className="block truncate font-medium">{actor?.display_name || a.handle}</span>
                        <span className="block font-mono text-meta text-fg-muted">@{a.handle}</span>
                      </span>
                    </span>
                  </td>
                  <td className="py-1.5 pr-3">
                    <span className="font-mono text-meta">{a.client}</span>
                    <span className="block text-meta text-fg-muted">{t(`agents.mode.${a.mode}`)}</span>
                  </td>
                  <td className="py-1.5 pr-3">
                    <Badge tone={a.sandbox ? "info" : "warning"}>{t(a.sandbox ? "agents.sandbox" : "agents.full")}</Badge>
                  </td>
                  <td className="tabular py-1.5 pr-3">{a.grants.length}</td>
                  <td className="py-1.5">
                    <span className="flex justify-end gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={<PanelRight className="size-4" />}
                        aria-label={t("agents.overviewOf", { handle: a.handle })}
                        onClick={() => dispatch({ type: "openWork", tab: { kind: "agent", agentId: a.agent_id, ref: "" } })}
                      >
                        <span className="hidden sm:inline">{t("agents.overview")}</span>
                      </Button>
                      <Link
                        to={settingsSearch(a.handle)}
                        aria-label={t("agents.settingsOf", { handle: a.handle })}
                        className="inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-ui text-fg-muted hover:bg-surface-sunken hover:text-fg"
                      >
                        <Settings aria-hidden className="size-4" />
                        <span className="hidden sm:inline">{t("agents.settings")}</span>
                      </Link>
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
