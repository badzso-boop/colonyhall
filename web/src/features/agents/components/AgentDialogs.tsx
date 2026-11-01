import { useSearchParams } from "react-router";
import { useHosts } from "@/features/hosts";
import { useAgentActors, useAgentInstructions } from "../api";
import { emptyDraft, fromAgent } from "../creation";
import { AgentSettingsDialog, settingsTabs, type SettingsTab } from "./AgentSettingsDialog";
import { CreateAgentDialog } from "./CreateAgentDialog";

const params = ["agent", "settings", "new", "host", "copy"];

/** "Másolat új agentként": the creation dialog prefilled with an agent's configuration. */
function CopyDialog({ handle, onClose }: { handle: string; onClose: () => void }) {
  const actors = useAgentActors(true);
  const hosts = useHosts();
  const actor = (actors.data ?? []).filter((a) => a.handle === handle && a.kind === "agent").sort((a, b) => Number(Boolean(a.archived_at)) - Number(Boolean(b.archived_at)))[0];
  const config = hosts.data?.flatMap((h) => h.agents).find((a) => a.agent_id === actor?.id);
  const instructions = useAgentInstructions(actor?.id ?? "", Boolean(actor));
  if (!actor || hosts.isPending || instructions.isPending) return null;
  return <CreateAgentDialog initial={fromAgent(actor, config, instructions.data?.text ?? "")} onClose={onClose} />;
}

/**
 * The agent dialogs, addressable in the URL from anywhere (docs/handoff/33,
 * part 4): ?agent=<handle>&settings=<tab>, ?new=1[&host=<id>], ?new=1&copy=<handle>.
 */
export function AgentDialogs() {
  const [search, setSearch] = useSearchParams();
  const clear = () =>
    setSearch((prev) => {
      const next = new URLSearchParams(prev);
      params.forEach((p) => next.delete(p));
      return next;
    });
  const handle = search.get("agent");
  const tab = search.get("settings");
  if (search.get("new") === "1") {
    const copy = search.get("copy");
    if (copy) return <CopyDialog key={copy} handle={copy} onClose={clear} />;
    return <CreateAgentDialog initial={emptyDraft(search.get("host") ?? "")} onClose={clear} />;
  }
  if (handle && tab && (settingsTabs as readonly string[]).includes(tab))
    return (
      <AgentSettingsDialog
        key={handle}
        handle={handle}
        tab={tab as SettingsTab}
        onTab={(t) => setSearch((prev) => new URLSearchParams({ ...Object.fromEntries(prev), settings: t }), { replace: true })}
        onClose={clear}
        onCopy={() =>
          setSearch((prev) => {
            const next = new URLSearchParams(prev);
            params.forEach((p) => next.delete(p));
            next.set("new", "1");
            next.set("copy", handle);
            return next;
          })
        }
      />
    );
  return null;
}
