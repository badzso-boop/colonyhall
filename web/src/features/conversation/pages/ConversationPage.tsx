import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";
import { Hash, Lock } from "lucide-react";
import { useShortcut } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { EmptyState, Skeleton } from "@/shared/ui";
import { useActors, useSession } from "@/features/identity";
import { useChannels, useMembers } from "../api";
import { useAttachments } from "../attachments";
import { ActivityLine } from "../components/ActivityLine";
import { Composer } from "../components/Composer";
import { DropZone } from "../components/DropZone";
import { Feed } from "../components/Feed";
import { conversationTitle, type Actor, type Channel } from "../model";
import { conversationAgent, openAgent, openFiles, openTerminals } from "../workPanes";

function Conversation({ channel, actors, meId }: { channel: Channel; actors: Map<string, Actor>; meId: string }) {
  const { t } = useTranslation("conversation");
  const [, dispatch] = usePanes();
  const session = useSession();
  const members = useMembers(channel.id);
  const tray = useAttachments(channel.id);
  const title = conversationTitle(channel, actors, meId);
  const memberIds = useMemo(() => new Set((members.data ?? []).map((m) => m.actor_id)), [members.data]);
  const agents = useMemo(
    () => (members.data ?? []).filter((m) => m.kind === "agent").map((m) => actors.get(m.actor_id)).filter((a): a is Actor => a !== undefined),
    [members.data, actors],
  );
  const agent = conversationAgent(agents);
  const mine = members.data?.find((m) => m.actor_id === meId);
  // Read only: an observer, or a member without write access (admins observing a DM can write).
  const readOnly = session.data?.actor.role === "observer" || (mine !== undefined && !mine.can_write);

  useShortcut("mod+shift+f", "conversation", "files", () => agent && dispatch(openFiles(agent.id)), Boolean(agent));
  // Ctrl+Shift+T is the browser's "reopen tab" and cannot be taken: X for terminals.
  useShortcut("mod+shift+x", "conversation", "terminals", () => agent && dispatch(openTerminals(agent.id)), Boolean(agent));
  useShortcut("r", "conversation", "replyThread");

  const handlers = {
    onOpenThread: (rootId: string) => dispatch({ type: "openThread", rootId }),
    onMention: (handle: string) => {
      const a = [...actors.values()].find((x) => x.handle === handle);
      if (a?.kind === "agent") dispatch(openAgent(a.id));
    },
    onFileClick: (author: Actor | undefined, link: Parameters<typeof openFiles>[1]) => {
      if (author?.kind === "agent") dispatch(openFiles(author.id, link));
    },
  };

  return (
    <DropZone onFiles={tray.add} className="flex h-full min-h-0 flex-col">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-surface px-4">
        {channel.kind === "private" ? <Lock aria-hidden className="size-4 text-fg-subtle" /> : null}
        {channel.kind === "public" ? <Hash aria-hidden className="size-4 text-fg-subtle" /> : null}
        <h1 className="text-title font-semibold whitespace-nowrap">{title}</h1>
        {channel.description ? <p className="min-w-0 flex-1 truncate text-ui text-fg-muted">{channel.description}</p> : null}
      </header>
      <Feed key={channel.id} channel={channel} actors={actors} meId={meId} handlers={handlers} />
      <ActivityLine channelId={channel.id} actors={actors} />
      <Composer
        channelId={channel.id}
        placeholder={t("composer.placeholder", { name: channel.kind === "dm" ? title : `#${title}` })}
        memberIds={memberIds}
        tray={tray}
        readOnly={readOnly}
        onOpenFiles={agent ? () => dispatch(openFiles(agent.id)) : undefined}
        onOpenTerminals={agent ? () => dispatch(openTerminals(agent.id)) : undefined}
      />
    </DropZone>
  );
}

export default function ConversationPage() {
  const { channelId = "" } = useParams();
  const { t } = useTranslation("conversation");
  const channels = useChannels();
  const actors = useActors();
  const session = useSession();
  const channel = channels.data?.find((c) => c.id === channelId);
  if (channels.data && !channel) return <EmptyState title={t("notFound")} />;
  if (!channel || !actors.data || !session.data) return <Skeleton className="m-4 h-6 w-1/3" />;
  return <Conversation key={channel.id} channel={channel} actors={actors.data} meId={session.data.actor.id} />;
}
