import { useTranslation } from "react-i18next";
import { Bot, FolderOpen, SquareTerminal, X } from "lucide-react";
import { cn } from "@/shared/lib";
import { tabKey, usePanes, type WorkTab } from "@/shared/panes";
import { IconButton } from "@/shared/ui";
import { AgentOverviewPane } from "@/features/agents";
import { FileBrowserPane } from "@/features/files";
import { useActors } from "@/features/identity";
import { TerminalPane } from "@/features/terminals";

const icons = { terminal: SquareTerminal, files: FolderOpen, agent: Bot } as const;

function TabContent({ tab }: { tab: WorkTab }) {
  switch (tab.kind) {
    case "agent":
      return <AgentOverviewPane agentId={tab.agentId} terminals={<TerminalPane agentId={tab.agentId} name="" />} />;
    case "terminal":
      return <TerminalPane agentId={tab.agentId} name={tab.ref} />;
    case "files":
      return <FileBrowserPane agentId={tab.agentId} path={tab.ref} />;
  }
}

/** The one work pane, with a tab per terminal, file browser or agent overview (rule 4). */
export function WorkPane() {
  const { t } = useTranslation("layout");
  const [state, dispatch] = usePanes();
  const actors = useActors();
  const active = state.tabs[state.active];
  if (!active) return null;
  const label = (tab: WorkTab) => {
    const handle = actors.data?.get(tab.agentId)?.handle ?? "?";
    const name = tab.ref ? tab.ref.split("/").pop() : t(`common:workKinds.${tab.kind}`);
    return `@${handle} · ${name}`;
  };
  return (
    <section aria-label={t("panes.work")} className="flex h-full min-h-0 flex-col bg-surface">
      <div role="tablist" className="flex h-10 shrink-0 items-end gap-0.5 overflow-x-auto border-b border-border px-1">
        {state.tabs.map((tab, i) => {
          const Icon = icons[tab.kind];
          const selected = i === state.active;
          return (
            <div
              key={tabKey(tab)}
              className={cn(
                "group flex h-9 max-w-56 shrink-0 items-center gap-1.5 rounded-t-md border border-b-0 pr-1 pl-2.5 text-meta",
                selected ? "border-border bg-bg text-fg" : "border-transparent text-fg-muted hover:text-fg",
              )}
              onAuxClick={(e) => e.button === 1 && dispatch({ type: "closeTab", index: i })}
            >
              <button
                type="button"
                role="tab"
                aria-selected={selected}
                onClick={() => dispatch({ type: "selectTab", index: i })}
                className="flex min-w-0 items-center gap-1.5"
              >
                <Icon aria-hidden className="size-3.5 shrink-0" />
                <span className="truncate font-mono">{label(tab)}</span>
              </button>
              <IconButton size="sm" label={t("panes.closeTab", { name: label(tab) })} icon={<X className="size-3.5" />} onClick={() => dispatch({ type: "closeTab", index: i })} />
            </div>
          );
        })}
        <IconButton className="mb-1 ml-auto" size="sm" label={t("panes.closeWork")} icon={<X className="size-4" />} onClick={() => dispatch({ type: "closeWork" })} />
      </div>
      <div role="tabpanel" className="min-h-0 flex-1">
        <TabContent key={tabKey(active)} tab={active} />
      </div>
    </section>
  );
}
