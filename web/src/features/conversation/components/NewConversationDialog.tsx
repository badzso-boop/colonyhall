// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Hash, Lock, Search } from "lucide-react";
import { useActors, useSession, type Actor } from "@/features/identity";
import { api, unwrap } from "@/shared/api";
import { cn, fuzzyScore, hueVar } from "@/shared/lib";
import { Avatar, Button, Dialog, Input, Textarea, toast } from "@/shared/ui";
import { conversationKeys } from "../api";
import { MAX_DESCRIPTION, validChannelName } from "../channel";
import { MemberChecklist } from "./MemberPicker";

export type NewConversationKind = "channel" | "dm" | "agentDm";

/** People to message: humans for a DM, agents for an AI conversation; never yourself. */
export function candidates(actors: Actor[], kind: "dm" | "agentDm", meId: string, query: string): Actor[] {
  const q = query.trim().toLowerCase();
  return actors
    .filter((a) => a.id !== meId && !a.archived_at && (kind === "dm" ? a.kind === "human" : a.kind === "agent"))
    .map((a) => ({ a, score: q ? Math.max(fuzzyScore(a.handle, q), fuzzyScore(a.display_name, q)) : 1 }))
    .filter((x) => x.score > 0)
    .sort((x, y) => y.score - x.score || x.a.handle.localeCompare(y.a.handle))
    .map((x) => x.a);
}

/** The sidebar's "+": a new channel, or a DM with a person or an agent. */
export function NewConversationDialog({ kind, meId, onClose }: { kind: NewConversationKind; meId: string; onClose: () => void }) {
  const { t } = useTranslation("conversation");
  const navigate = useNavigate();
  const qc = useQueryClient();
  const actors = useActors();
  const [query, setQuery] = useState("");
  const [name, setName] = useState("");
  const [isPrivate, setPrivate] = useState(false);
  const [description, setDescription] = useState("");
  const [members, setMembers] = useState<string[]>([]);
  const role = useSession().data?.actor.role;
  // Adding members is for admins (the server refuses it too).
  const admin = role === "owner" || role === "admin";
  const done = (id: string) => {
    void qc.invalidateQueries({ queryKey: conversationKeys.channels });
    onClose();
    navigate(`/c/${id}`);
  };
  const openDm = useMutation({
    mutationFn: async (actorId: string) => unwrap(await api.POST("/dms", { body: { actor_ids: [actorId] } })),
    onSuccess: (ch) => done(ch.id),
    onError: (e) => toast(e.message, { tone: "danger" }),
  });
  const createChannel = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/channels", {
          body: {
            name: name.trim(),
            kind: isPrivate ? "private" : "public",
            ...(description.trim() ? { description: description.trim() } : {}),
            ...(members.length ? { member_ids: members } : {}),
          },
        }),
      ),
    onSuccess: (ch) => done(ch.id),
    onError: (e) => toast(e.message, { tone: "danger" }),
  });
  const list = useMemo(() => (kind === "channel" ? [] : candidates([...(actors.data?.values() ?? [])], kind, meId, query)), [actors.data, kind, meId, query]);
  const title = t(kind === "channel" ? "actions.newChannel" : kind === "dm" ? "actions.newDm" : "actions.newAgentDm");

  if (kind === "channel")
    return (
      <Dialog
        open
        onOpenChange={(o) => !o && onClose()}
        title={title}
        closeLabel={t("new.close")}
        footer={
          <>
            <Button variant="ghost" onClick={onClose}>
              {t("new.cancel")}
            </Button>
            <Button variant="primary" disabled={!validChannelName(name.trim()) || description.length > MAX_DESCRIPTION} loading={createChannel.isPending} onClick={() => createChannel.mutate()}>
              {t("new.create")}
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <label className="flex flex-col gap-1">
            <span className="text-meta font-semibold text-fg-muted">{t("new.name")}</span>
            <Input autoFocus value={name} onChange={(e) => setName(e.target.value.toLowerCase())} maxLength={32} placeholder="pl. trader" aria-invalid={name !== "" && !validChannelName(name.trim())} />
            {name !== "" && !validChannelName(name.trim()) ? <span className="text-meta text-danger">{t("channel.nameRule")}</span> : null}
          </label>
          <div role="radiogroup" aria-label={t("new.visibility")} className="flex gap-2">
            {[false, true].map((p) => (
              <button
                key={String(p)}
                type="button"
                role="radio"
                aria-checked={isPrivate === p}
                onClick={() => setPrivate(p)}
                className={cn("flex flex-1 items-center gap-2 rounded-md border p-2 text-left text-ui", isPrivate === p ? "border-accent bg-accent-soft" : "border-border")}
              >
                {p ? <Lock aria-hidden className="size-4" /> : <Hash aria-hidden className="size-4" />}
                <span>
                  <span className="block font-semibold">{t(p ? "new.private" : "new.public")}</span>
                  <span className="block text-meta text-fg-muted">{t(p ? "new.privateHint" : "new.publicHint")}</span>
                </span>
              </button>
            ))}
          </div>
          <label className="flex flex-col gap-1">
            <span className="flex text-meta font-semibold text-fg-muted">
              <span className="flex-1">{t("channel.description")}</span>
              <span className={description.length > MAX_DESCRIPTION ? "text-danger" : "tabular-nums"}>
                {description.length}/{MAX_DESCRIPTION}
              </span>
            </span>
            <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
          </label>
          {admin ? (
            <section aria-label={t("channel.members.initial")} className="flex flex-col gap-1">
              <span className="text-meta font-semibold text-fg-muted">{t("channel.members.initial")}</span>
              <MemberChecklist
                actors={[...(actors.data?.values() ?? [])]}
                selected={members}
                exclude={[meId]}
                onToggle={(id) => setMembers((m) => (m.includes(id) ? m.filter((x) => x !== id) : [...m, id]))}
              />
            </section>
          ) : null}
        </div>
      </Dialog>
    );

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={title} closeLabel={t("new.close")}>
      <div className="flex flex-col gap-2">
        <label className="relative">
          <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
          <Input autoFocus type="search" aria-label={t("new.search")} placeholder={t("new.search")} value={query} onChange={(e) => setQuery(e.target.value)} className="pl-8" />
        </label>
        <ul aria-label={t("new.people")} className="flex max-h-80 flex-col gap-px overflow-y-auto">
          {list.map((a) => (
            <li key={a.id}>
              <button
                type="button"
                disabled={openDm.isPending}
                onClick={() => openDm.mutate(a.id)}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left hover:bg-surface-sunken"
              >
                <Avatar kind={a.kind === "agent" ? "agent" : "human"} name={a.display_name || a.handle} emoji={a.avatar_emoji} size="sm" ringColor={hueVar(a.effective_color)} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{a.display_name || a.handle}</span>
                  <span className="block truncate font-mono text-meta text-fg-muted">@{a.handle}</span>
                </span>
              </button>
            </li>
          ))}
          {actors.data && list.length === 0 ? <li className="px-2 py-3 text-meta text-fg-subtle">{t("new.none")}</li> : null}
        </ul>
      </div>
    </Dialog>
  );
}
