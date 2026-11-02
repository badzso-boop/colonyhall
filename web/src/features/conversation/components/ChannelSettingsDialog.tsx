import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Archive, Search } from "lucide-react";
import { useActivity } from "@/features/agents";
import { api, unwrap } from "@/shared/api";
import { hueVar } from "@/shared/lib";
import { Avatar, Badge, Button, Dialog, Input, LargeDialog, Markdown, Notice, Textarea, toast } from "@/shared/ui";
import { useAddMembers, useMembers, useRemoveMember, useSetArchived, useUpdateChannel } from "../api";
import { channelTabs, MAX_DESCRIPTION, orderMembers, validChannelName, type ChannelTab } from "../channel";
import type { Actor, Channel } from "../model";
import { MemberPicker } from "./MemberPicker";
import { ProfileTrigger } from "./ProfilePopover";

type Props = { channel: Channel; actors: Map<string, Actor>; admin: boolean; tab: ChannelTab; onTab: (t: ChannelTab) => void; onClose: () => void };

/**
 * A channel's settings (docs/handoff/34, 1.3), the large Dialog of round 33.
 * Admins edit; everyone else gets the read-only "Csatorna adatai".
 */
export function ChannelSettingsDialog({ channel, actors, admin, tab, onTab, onClose }: Props) {
  const { t } = useTranslation("conversation");
  const [name, setName] = useState(channel.name);
  const [description, setDescription] = useState(channel.description ?? "");
  const update = useUpdateChannel(channel.id);
  const dirty = name !== channel.name || description !== (channel.description ?? "");
  const nameOk = validChannelName(name);
  const tabs = channelTabs.filter((id) => admin || id === "altalanos" || id === "tagok");
  const active = tabs.includes(tab) ? tab : "altalanos";
  const save = () =>
    update.mutate(
      { ...(name !== channel.name ? { name } : {}), ...(description !== (channel.description ?? "") ? { description } : {}) },
      { onSuccess: () => toast(t("channel.saved")), onError: (e) => toast(e.message, { tone: "danger" }) },
    );
  return (
    <LargeDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={admin ? t("channel.settingsTitle", { name: channel.name }) : t("channel.infoTitle", { name: channel.name })}
      tabs={tabs.map((id) => ({ id, label: t(`channel.tabs.${id}`) }))}
      active={active}
      onSelect={(id) => onTab(id as ChannelTab)}
      navLabel={t("channel.tabsLabel")}
      closeLabel={t("new.close")}
      backLabel={t("channel.back")}
      banner={channel.archived_at ? <Notice icon={<Archive className="size-4" />}>{t("channel.archivedNotice")}</Notice> : null}
      beforeClose={() => !dirty || window.confirm(t("channel.discard"))}
      footer={
        admin && active === "altalanos" ? (
          <Button variant="primary" disabled={!dirty || !nameOk || description.length > MAX_DESCRIPTION} loading={update.isPending} onClick={save}>
            {t("channel.save")}
          </Button>
        ) : undefined
      }
    >
      {active === "altalanos" ? (
        <General channel={channel} admin={admin} name={name} setName={setName} nameOk={nameOk} description={description} setDescription={setDescription} />
      ) : null}
      {active === "tagok" ? <Members channel={channel} actors={actors} admin={admin} /> : null}
      {active === "tomorites" ? <Compaction channel={channel} /> : null}
      {active === "archivalas" ? <ArchiveTab channel={channel} /> : null}
    </LargeDialog>
  );
}

function General({ channel, admin, name, setName, nameOk, description, setDescription }: {
  channel: Channel;
  admin: boolean;
  name: string;
  setName: (v: string) => void;
  nameOk: boolean;
  description: string;
  setDescription: (v: string) => void;
}) {
  const { t } = useTranslation("conversation");
  if (!admin)
    return (
      <div className="flex flex-col gap-3">
        <h3 className="text-meta font-semibold text-fg-muted">{t("channel.description")}</h3>
        {channel.description ? <Markdown text={channel.description} copyLabel={t("message.copy")} copiedLabel={t("message.copied")} /> : <p className="text-fg-subtle">{t("channel.noDescription")}</p>}
      </div>
    );
  return (
    <div className="flex flex-col gap-4">
      {channel.kind !== "dm" ? (
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("new.name")}</span>
          <Input value={name} onChange={(e) => setName(e.target.value.toLowerCase())} aria-invalid={!nameOk} maxLength={32} />
          {!nameOk ? <span className="text-meta text-danger">{t("channel.nameRule")}</span> : null}
        </label>
      ) : null}
      <label className="flex flex-col gap-1">
        <span className="flex items-center text-meta font-semibold text-fg-muted">
          <span className="flex-1">{t("channel.description")}</span>
          <span className={description.length > MAX_DESCRIPTION ? "text-danger" : "tabular-nums"}>
            {description.length}/{MAX_DESCRIPTION}
          </span>
        </span>
        <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={4} />
      </label>
      <p className="text-meta text-fg-muted">{t("channel.descriptionHint")}</p>
      {description ? (
        <section aria-label={t("channel.preview")} className="rounded-md border border-border bg-surface-sunken p-3">
          <h3 className="mb-1 text-meta font-semibold text-fg-subtle">{t("channel.preview")}</h3>
          <Markdown text={description} copyLabel={t("message.copy")} copiedLabel={t("message.copied")} />
        </section>
      ) : null}
    </div>
  );
}

function Members({ channel, actors, admin }: { channel: Channel; actors: Map<string, Actor>; admin: boolean }) {
  const { t } = useTranslation("conversation");
  const members = useMembers(channel.id);
  const working = useActivity((s) => s.working);
  const add = useAddMembers(channel.id);
  const remove = useRemoveMember(channel.id);
  const [query, setQuery] = useState("");
  const [adding, setAdding] = useState(false);
  const [removing, setRemoving] = useState<Actor | null>(null);
  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = (members.data ?? [])
      .map((m) => ({ member: m, actor: actors.get(m.actor_id) }))
      .filter((r): r is { member: (typeof r)["member"]; actor: Actor } => r.actor !== undefined)
      .filter((r) => !q || r.actor.handle.includes(q) || r.actor.display_name.toLowerCase().includes(q));
    return orderMembers(list);
  }, [members.data, actors, query]);
  const dm = channel.kind === "dm";
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <label className="relative flex-1">
          <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
          <Input type="search" aria-label={t("channel.members.search")} placeholder={t("channel.members.search")} value={query} onChange={(e) => setQuery(e.target.value)} className="pl-8" />
        </label>
        {admin && !dm ? (
          <Button onClick={() => setAdding(true)}>{t("channel.members.add")}</Button>
        ) : null}
      </div>
      <p className="text-meta text-fg-muted">{t("channel.members.count", { count: members.data?.length ?? 0 })}</p>
      <ul aria-label={t("channel.tabs.tagok")} className="flex flex-col gap-px">
        {rows.map(({ actor }) => {
          const here = (working[actor.id] ?? []).some((p) => p.channelId === channel.id);
          return (
            <li key={actor.id} className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-surface-sunken">
              <ProfileTrigger actorId={actor.id} className="flex min-w-0 flex-1 items-center gap-2 text-left">
                <Avatar kind={actor.kind === "agent" ? "agent" : "human"} name={actor.display_name || actor.handle} emoji={actor.avatar_emoji} size="sm" ringColor={hueVar(actor.effective_color)} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{actor.display_name || actor.handle}</span>
                  <span className="block truncate font-mono text-meta text-fg-muted">@{actor.handle}</span>
                </span>
              </ProfileTrigger>
              {actor.kind === "agent" && actor.state !== "active" ? <Badge>{actor.state}</Badge> : null}
              {here ? <Badge tone="accent">{t("channel.members.workingHere")}</Badge> : null}
              {admin && !dm ? (
                <Button size="sm" variant="ghost" onClick={() => setRemoving(actor)}>
                  {t("channel.members.remove")}
                </Button>
              ) : null}
            </li>
          );
        })}
      </ul>
      {adding ? (
        <MemberPicker
          actors={[...actors.values()]}
          members={(members.data ?? []).map((m) => m.actor_id)}
          pending={add.isPending}
          onClose={() => setAdding(false)}
          onAdd={(ids) =>
            add.mutate(ids, {
              onSuccess: () => {
                setAdding(false);
                toast(t("channel.members.added", { count: ids.length }));
              },
              onError: (e) => toast(e.message, { tone: "danger" }),
            })
          }
        />
      ) : null}
      {removing ? (
        <Dialog
          open
          onOpenChange={(o) => !o && setRemoving(null)}
          title={t("channel.members.removeTitle", { handle: removing.handle })}
          closeLabel={t("new.close")}
          footer={
            <>
              <Button variant="ghost" onClick={() => setRemoving(null)}>
                {t("new.cancel")}
              </Button>
              <Button
                variant="danger"
                loading={remove.isPending}
                onClick={() =>
                  remove.mutate(removing.id, {
                    onSuccess: () => setRemoving(null),
                    onError: (e) => toast(e.message, { tone: "danger" }),
                  })
                }
              >
                {t("channel.members.remove")}
              </Button>
            </>
          }
        >
          <p>{t(removing.kind === "agent" ? "channel.members.removeAgent" : "channel.members.removeHuman", { handle: removing.handle, channel: channel.name })}</p>
        </Dialog>
      ) : null}
    </div>
  );
}

function Compaction({ channel }: { channel: Channel }) {
  const { t } = useTranslation("conversation");
  const status = useQuery({
    queryKey: ["conversation", "compaction", channel.id],
    queryFn: async () => unwrap(await api.GET("/channels/{channelId}/compaction", { params: { path: { channelId: channel.id } } })),
  });
  const settings = useQuery({ queryKey: ["compaction", "settings"], queryFn: async () => unwrap(await api.GET("/settings/compaction")) });
  const [value, setValue] = useState(channel.compact_threshold_tokens ? String(channel.compact_threshold_tokens) : "");
  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PATCH("/channels/{channelId}/compaction", {
          params: { path: { channelId: channel.id } },
          body: { compact_threshold_tokens: value.trim() ? Number(value) : null },
        }),
      ),
    onSuccess: () => {
      toast(t("channel.saved"));
      void status.refetch();
    },
    onError: (e) => toast(e.message, { tone: "danger" }),
  });
  const now = useMutation({
    mutationFn: async () => unwrap(await api.POST("/channels/{channelId}/compactions", { params: { path: { channelId: channel.id } }, body: {} })),
    onSuccess: () => toast(t("channel.compaction.started")),
    onError: (e) => toast(e.message, { tone: "danger" }),
  });
  const n = Number(value);
  const valid = !value.trim() || (Number.isInteger(n) && n >= 20000 && n <= 2000000);
  return (
    <div className="flex flex-col gap-4">
      <label className="flex flex-col gap-1">
        <span className="text-meta font-semibold text-fg-muted">{t("channel.compaction.threshold")}</span>
        <Input inputMode="numeric" value={value} onChange={(e) => setValue(e.target.value.replace(/[^0-9]/g, ""))} placeholder={String(settings.data?.compact_threshold_tokens ?? "")} aria-invalid={!valid} />
        <span className="text-meta text-fg-muted">{t("channel.compaction.default", { tokens: settings.data?.compact_threshold_tokens ?? "–" })}</span>
      </label>
      {status.data ? (
        <p className="text-meta text-fg-muted">{t("channel.compaction.since", { tokens: status.data.approx_tokens_since, messages: status.data.messages_since })}</p>
      ) : null}
      <div className="flex gap-2">
        <Button disabled={!valid} loading={save.isPending} onClick={() => save.mutate()}>
          {t("channel.save")}
        </Button>
        <Button variant="ghost" disabled={Boolean(channel.archived_at) || status.data?.has_pending_compaction} loading={now.isPending} onClick={() => now.mutate()}>
          {t("channel.compaction.now")}
        </Button>
      </div>
    </div>
  );
}

function ArchiveTab({ channel }: { channel: Channel }) {
  const { t } = useTranslation("conversation");
  const setArchived = useSetArchived(channel.id);
  const [confirm, setConfirm] = useState(false);
  const archived = Boolean(channel.archived_at);
  const run = (archive: boolean) =>
    setArchived.mutate(archive, {
      onSuccess: () => {
        setConfirm(false);
        toast(t(archive ? "channel.archive.done" : "channel.archive.restored"));
      },
      onError: (e) => toast(e.message, { tone: "danger" }),
    });
  return (
    <div className="flex flex-col gap-3">
      <p>{t(archived ? "channel.archive.archivedBody" : "channel.archive.body")}</p>
      {archived ? (
        <Button className="self-start" loading={setArchived.isPending} onClick={() => run(false)}>
          {t("channel.archive.restore")}
        </Button>
      ) : (
        <Button className="self-start" variant="danger" onClick={() => setConfirm(true)}>
          {t("channel.archive.action")}
        </Button>
      )}
      {confirm ? (
        <Dialog
          open
          onOpenChange={(o) => !o && setConfirm(false)}
          title={t("channel.archive.confirmTitle", { name: channel.name })}
          closeLabel={t("new.close")}
          footer={
            <>
              <Button variant="ghost" onClick={() => setConfirm(false)}>
                {t("new.cancel")}
              </Button>
              <Button variant="danger" loading={setArchived.isPending} onClick={() => run(true)}>
                {t("channel.archive.action")}
              </Button>
            </>
          }
        >
          <p>{t("channel.archive.confirmBody")}</p>
        </Dialog>
      ) : null}
    </div>
  );
}
