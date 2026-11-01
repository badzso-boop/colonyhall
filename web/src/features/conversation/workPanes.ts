import type { PaneAction } from "@/shared/panes";
import type { FileLink } from "@/shared/lib";
import type { Actor } from "./model";

/** The open-a-pane actions the conversation view uses. */
export const openAgent = (agentId: string): PaneAction => ({ type: "openWork", tab: { kind: "agent", agentId, ref: "" } });
export const openFiles = (agentId: string, link?: FileLink): PaneAction => ({
  type: "openWork",
  tab: { kind: "files", agentId, ref: link ? link.path + (link.lines ? `:${link.lines}` : "") : "" },
});
export const openTerminals = (agentId: string, name = ""): PaneAction => ({ type: "openWork", tab: { kind: "terminal", agentId, ref: name } });

/** The agent a conversation's Files/Terminal buttons act on: the DM's agent, or the channel's only agent. */
export function conversationAgent(agents: Actor[]): Actor | undefined {
  return agents.length === 1 ? agents[0] : undefined;
}
