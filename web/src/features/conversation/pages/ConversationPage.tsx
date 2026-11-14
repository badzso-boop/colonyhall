// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams, useSearchParams } from "react-router";
import { useShortcut } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { EmptyState, Skeleton, toast } from "@/shared/ui";
import { GrantBar } from "@/features/approvals";
import { useActors, useSession } from "@/features/identity";
import { useChannels, useMembers } from "../api";
import { useAttachments } from "../attachments";
import { isChannelTab, type ChannelTab } from "../channel";
import { ActivityLine } from "../components/ActivityLine";
import { ArchivedBar, ChannelHeader } from "../components/ChannelHeader";
import { ChannelSettingsDialog } from "../components/ChannelSettingsDialog";
import { Composer } from "../components/Composer";
import { DropZone } from "../components/DropZone";
import { Feed } from "../components/Feed";
import { conversationTitle, type Actor, type Channel } from "../model";
import { useViewer } from "../viewer";
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
  const role = session.data?.actor.role;
  const admin = role === "owner" || role === "admin";
  const [params, setParams] = useSearchParams();
  const settingsTab = params.get("channel-settings");
  const setSettings = (tab: ChannelTab | null) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p);
        if (tab) next.set("channel-settings", tab);
        else next.delete("channel-settings");
        return next;
      },
      { replace: tab !== null && settingsTab !== null },
    );
  const viewer = useViewer(!readOnly);

  useShortcut("mod+shift+f", "conversation", "files", () => agent && dispatch(openFiles(agent.id)), Boolean(agent));
  // Ctrl+Shift+T is the browser's "reopen tab" and cannot be taken: X for terminals.
  useShortcut("mod+shift+x", "conversation", "terminals", () => agent && dispatch(openTerminals(agent.id)), Boolean(agent));
  useShortcut("r", "conversation", "replyThread");
  // Round 55, part F: handled by the focused message itself, listed here for the help.
  useShortcut("s", "conversation", "saveMessage");
  useShortcut("l", "conversation", "copyLink");
  useShortcut("f", "conversation", "forwardMessage");
  useShortcut("p", "conversation", "pinMessage");

  const handlers = {
    onOpenThread: (rootId: string) => dispatch({ type: "openThread", rootId }),
    onMention: (handle: string) => {
      const a = [...actors.values()].find((x) => x.handle === handle);
      if (a?.kind === "agent") dispatch(openAgent(a.id));
    },
    onFileClick: (author: Actor | undefined, link: Parameters<typeof openFiles>[1]) => {
      if (author?.kind === "agent") dispatch(openFiles(author.id, link));
    },
    onOpenTerminal: (agentId: string, name: string) => dispatch(openTerminals(agentId, name)),
  };

  return (
    <DropZone onFiles={tray.add} className="flex h-full min-h-0 flex-col">
      <ChannelHeader channel={channel} title={title} actors={actors} admin={admin} onSettings={setSettings} />
      {viewer.human ? <GrantBar channelId={channel.id} isDm={channel.kind === "dm"} admin={viewer.admin} /> : null}
      <Feed key={channel.id} channel={channel} actors={actors} viewer={viewer} handlers={handlers} />
      <ActivityLine channelId={channel.id} actors={actors} />
      {channel.archived_at ? (
        <ArchivedBar channel={channel} admin={admin} />
      ) : (
      <Composer
        channelId={channel.id}
        placeholder={t("composer.placeholder", { name: channel.kind === "dm" ? title : `#${title}` })}
        memberIds={memberIds}
        tray={tray}
        readOnly={readOnly}
        onOpenFiles={agent ? () => dispatch(openFiles(agent.id)) : undefined}
        onOpenTerminals={agent ? () => dispatch(openTerminals(agent.id)) : undefined}
      />
      )}
      {isChannelTab(settingsTab) && channel.kind !== "dm" ? (
        <ChannelSettingsDialog key={channel.id} channel={channel} actors={actors} admin={admin} tab={settingsTab} onTab={setSettings} onClose={() => setSettings(null)} />
      ) : null}
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
  useLeftChannelToast(channelId, channel);
  if (channels.data && !channel) return <EmptyState title={t("notFound")} />;
  if (!channel || !actors.data || !session.data) return <Skeleton className="m-4 h-6 w-1/3" />;
  return <Conversation key={channel.id} channel={channel} actors={actors.data} meId={session.data.actor.id} />;
}

/**
 * The viewer was removed from the open channel (it disappears from their
 * list, or an admin stops being a member): a toast, and out to the start page.
 */
function useLeftChannelToast(channelId: string, channel: Channel | undefined) {
  const { t } = useTranslation("conversation");
  const navigate = useNavigate();
  const wasMember = useRef<{ id: string; member: boolean } | null>(null);
  useEffect(() => {
    const prev = wasMember.current?.id === channelId ? wasMember.current : null;
    wasMember.current = channel ? { id: channelId, member: channel.is_member } : prev;
    if (!prev?.member || channel?.is_member) return;
    if (channel?.kind === "dm") return;
    wasMember.current = null;
    toast(t("channel.removedYou"));
    navigate("/");
  }, [channelId, channel, navigate, t]);
}
