import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Hash, Lock, Search } from "lucide-react";
import { useActors, type Actor } from "@/features/identity";
import { api, unwrap } from "@/shared/api";
import { cn, fuzzyScore, hueVar } from "@/shared/lib";
import { Avatar, Button, Dialog, Input, toast } from "@/shared/ui";
import { conversationKeys } from "../api";

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
    mutationFn: async () => unwrap(await api.POST("/channels", { body: { name: name.trim(), kind: isPrivate ? "private" : "public" } })),
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
            <Button variant="primary" disabled={!name.trim()} loading={createChannel.isPending} onClick={() => createChannel.mutate()}>
              {t("new.create")}
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <label className="flex flex-col gap-1">
            <span className="text-meta font-semibold text-fg-muted">{t("new.name")}</span>
            <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} maxLength={80} placeholder="pl. trader" />
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
