import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Copy, Lock, ShieldAlert, Trash2 } from "lucide-react";
import { useHosts } from "@/features/hosts";
import { useSession } from "@/features/identity";
import { cn, hueVar } from "@/shared/lib";
import { Avatar, Badge, Button, Dialog, Input, LargeDialog, Markdown, Notice, Textarea, toast } from "@/shared/ui";
import { useAgentActors, useAgentInstructions, useAgentOverview, useArchiveAgent, useChangeAgentState, useSetLimits, useUpdateAgentProfile } from "../api";
import { sessionPresets } from "../creation";
import { ColorSwatches } from "./CreateAgentDialog";

export const settingsTabs = ["profil", "keretek", "konfiguracio", "utasitas", "veszelyes"] as const;
export type SettingsTab = (typeof settingsTabs)[number];

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="flex items-center gap-1 text-fg-muted">
        <Lock aria-hidden className="size-3" />
        {label}
      </dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}

type Profile = { description: string; avatarEmoji: string; color: string; statusText: string; timezone: string };

/**
 * An agent's settings (docs/handoff/33, part 4): the profile and limits may
 * change; the configuration and instructions are read-only, with "Másolat
 * új agentként" as the way to change them.
 */
export function AgentSettingsDialog({ handle, tab, onTab, onClose, onCopy }: { handle: string; tab: SettingsTab; onTab: (t: SettingsTab) => void; onClose: () => void; onCopy: () => void }) {
  const { t } = useTranslation("agents");
  const actors = useAgentActors(true);
  const hosts = useHosts();
  // The active agent with the handle, else the newest archived one.
  const actor = (actors.data ?? []).filter((a) => a.handle === handle && a.kind === "agent").sort((a, b) => Number(Boolean(a.archived_at)) - Number(Boolean(b.archived_at)))[0];
  const host = hosts.data?.find((h) => h.agents.some((a) => a.agent_id === actor?.id));
  const config = host?.agents.find((a) => a.agent_id === actor?.id);
  const instructions = useAgentInstructions(actor?.id ?? "", Boolean(actor));
  const overview = useAgentOverview(actor?.id ?? "", Boolean(actor) && tab === "keretek");
  const archived = Boolean(actor?.archived_at);
  const role = useSession().data?.actor.role;
  const admin = role === "owner" || role === "admin";
  // Members see the settings but change nothing (the server refuses it too).
  const readOnly = archived || !admin;

  const fromActor = (): Profile => ({
    description: actor?.description ?? "", avatarEmoji: actor?.avatar_emoji ?? "", color: actor?.color ?? "",
    statusText: actor?.status_text ?? "", timezone: actor?.timezone ?? "Europe/Budapest",
  });
  const [profile, setProfile] = useState<Profile>(fromActor);
  const [loadedFor, setLoadedFor] = useState(actor?.id);
  if (actor?.id !== loadedFor) {
    setLoadedFor(actor?.id);
    setProfile(fromActor());
  }
  const saved = fromActor();
  const changes = (Object.keys(profile) as (keyof Profile)[]).filter((k) => profile[k] !== saved[k]);
  const dirty = changes.length > 0;
  // "Közben módosult": the agent changed elsewhere while the dialog is open.
  // The first loaded version is the baseline; later differences mean a change elsewhere.
  const seen = useRef<string | null>(null);
  const [changedElsewhere, setChangedElsewhere] = useState(false);
  const savedKey = actor ? JSON.stringify(saved) : null;
  useEffect(() => {
    if (savedKey === null) return;
    if (seen.current === null) seen.current = savedKey;
    else if (seen.current !== savedKey) setChangedElsewhere(true);
  }, [savedKey]);

  const [limits, setLimits] = useState<{ session: number; daily: number } | null>(null);
  const curLimits = limits ?? { session: config?.max_session_tokens ?? 0, daily: config?.daily_token_limit ?? 0 };
  const [ask, setAsk] = useState<null | { next?: SettingsTab; close?: boolean }>(null);
  const [danger, setDanger] = useState<null | "delete" | "stop">(null);
  const [typed, setTyped] = useState("");
  const updateProfile = useUpdateAgentProfile();
  const setLimitsM = useSetLimits();
  const setState = useChangeAgentState();
  const archive = useArchiveAgent();

  if (!actor) return null;
  const guard = (next: { next?: SettingsTab; close?: boolean }) => {
    if (tab === "profil" && dirty) setAsk(next);
    else if (next.close) onClose();
    else if (next.next) onTab(next.next);
  };
  const saveProfile = () =>
    updateProfile.mutate(
      { id: actor.id, description: profile.description, avatar_emoji: profile.avatarEmoji, color: profile.color, status_text: profile.statusText, timezone: profile.timezone },
      {
        onSuccess: () => {
          seen.current = JSON.stringify(profile);
          toast(t("settings.saved"));
        },
        onError: (e) => toast(e.message, { tone: "danger" }),
      },
    );

  const header = (
    <div className="flex items-center gap-2 text-ui">
      <Avatar kind="agent" name={actor.display_name || actor.handle} emoji={actor.avatar_emoji} size="md" ringColor={hueVar(actor.effective_color)} />
      <span className="font-semibold">{actor.display_name}</span>
      <span className="font-mono text-meta text-fg-muted">@{actor.handle}</span>
      {host ? (
        <span className="flex items-center gap-1 rounded-full bg-surface-sunken px-2 py-0.5 text-meta" title={host.name}>
          <span aria-hidden className="size-2 rounded-full" style={{ background: hueVar(host.color) }} />
          {host.name}
        </span>
      ) : null}
      {archived ? <Badge tone="neutral">{t("state.archived")}</Badge> : null}
    </div>
  );

  const body: Record<SettingsTab, ReactNode> = {
    profil: (
      <fieldset disabled={readOnly} className="grid gap-3 sm:grid-cols-2">
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("create.displayName")}</span>
          <Input value={actor.display_name} readOnly aria-readonly />
          <span className="text-meta text-fg-subtle">{t("settings.displayNameHint")}</span>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("create.emoji")}</span>
          <Input value={profile.avatarEmoji} onChange={(e) => setProfile({ ...profile, avatarEmoji: e.target.value })} className="w-24" />
        </label>
        <div className="flex flex-col gap-1 sm:col-span-2">
          <span className="text-meta font-semibold text-fg-muted">{t("create.color")}</span>
          <ColorSwatches value={profile.color} onChange={(color) => setProfile({ ...profile, color })} inherit="" />
        </div>
        <label className="flex flex-col gap-1 sm:col-span-2">
          <span className="text-meta font-semibold text-fg-muted">{t("create.description")}</span>
          <Textarea rows={2} maxLength={300} value={profile.description} onChange={(e) => setProfile({ ...profile, description: e.target.value })} />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("settings.status")}</span>
          <Input maxLength={80} value={profile.statusText} onChange={(e) => setProfile({ ...profile, statusText: e.target.value })} />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("settings.timezone")}</span>
          <Input value={profile.timezone} onChange={(e) => setProfile({ ...profile, timezone: e.target.value })} />
        </label>
        {dirty ? (
          <p className="text-meta text-fg-muted sm:col-span-2">
            {t("settings.whatChanges")}: {changes.map((c) => t(`settings.fields.${c}`)).join(", ")}
          </p>
        ) : null}
      </fieldset>
    ),
    keretek: (
      <fieldset disabled={readOnly || !config} className="flex flex-col gap-4">
        {!config ? <Notice>{t("settings.noLimits")}</Notice> : null}
        <div className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("create.sessionLimit")}</span>
          <div role="radiogroup" aria-label={t("create.sessionLimit")} className="flex flex-wrap gap-1">
            {sessionPresets.map((n) => (
              <button
                key={n}
                type="button"
                role="radio"
                aria-checked={curLimits.session === n}
                onClick={() => setLimits({ ...curLimits, session: n })}
                className={cn("rounded-md border px-2.5 py-1 text-ui", curLimits.session === n ? "border-accent bg-accent-soft font-semibold" : "border-border")}
              >
                {n ? `${n / 1000}k` : t("create.noLimit")}
              </button>
            ))}
          </div>
        </div>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("create.dailyLimit")}</span>
          <Input type="number" min={0} value={curLimits.daily} onChange={(e) => setLimits({ ...curLimits, daily: Number(e.target.value) })} className="w-48" />
          <span className="text-meta text-fg-subtle">{t("settings.today", { tokens: overview.data?.usage?.today_tokens ?? 0 })}</span>
        </label>
      </fieldset>
    ),
    konfiguracio: (
      <div className="flex flex-col gap-4">
        <Notice
          action={
            admin ? (
              <Button size="sm" icon={<Copy className="size-3.5" />} onClick={onCopy}>
                {t("settings.copy")}
              </Button>
            ) : undefined
          }
        >
          {t("settings.fixedInfo")}
        </Notice>
        {config && host ? (
          <>
            <section>
              <h3 className="mb-1 font-semibold">{t("create.steps.machine")}</h3>
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-ui">
                <Row label={t("create.host")}>{host.name}</Row>
                <Row label={t("create.client")}>{config.client}</Row>
                <Row label={t("create.model")}>
                  {config.model || "–"} / {config.work_model || "–"}
                </Row>
                <Row label={t("create.mode")}>{config.mode}</Row>
                <Row label={t("create.allowWork")}>{config.allow_work ? t("yes") : t("no")}</Row>
                <Row label={t("create.permission")}>{config.permission_mode}</Row>
              </dl>
            </section>
            <section>
              <h3 className="mb-1 font-semibold">{t("create.steps.access")}</h3>
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-ui">
                <Row label={t("create.access")}>
                  {config.sandbox ? (
                    t("create.sandbox")
                  ) : (
                    <span className="inline-flex items-center gap-1 text-warning">
                      <ShieldAlert aria-hidden className="size-3.5" />
                      {t("create.fullAccess")}
                    </span>
                  )}
                </Row>
                <Row label={t("create.rootDir")}>
                  <span className="font-mono text-meta">{config.root_dir || "–"}</span>
                </Row>
                <Row label={t("create.worktree")}>
                  <span className="font-mono text-meta">{config.worktree_repo || "–"}</span>
                </Row>
                <Row label={t("create.grants")}>
                  <ul className="font-mono text-meta">
                    {config.grants.map((g) => (
                      <li key={g.id}>
                        {g.path} · {t(`create.${g.mode}`)}
                      </li>
                    ))}
                    {config.grants.length === 0 ? <li>–</li> : null}
                  </ul>
                </Row>
                <Row label={t("create.approvals")}>{config.approvals}</Row>
                <Row label={t("create.preview")}>{config.allow_preview ? t("yes") : t("no")}</Row>
                <Row label={t("create.webRead")}>{config.allow_web_read ? t("yes") : t("no")}</Row>
              </dl>
            </section>
          </>
        ) : (
          <p className="text-ui text-fg-muted">{archived ? t("settings.archivedConfig") : t("create.apiKey")}</p>
        )}
      </div>
    ),
    utasitas: (
      <div className="flex flex-col gap-4">
        <section>
          <h3 className="mb-1 flex items-center gap-1.5 font-semibold">
            <Lock aria-hidden className="size-3.5" />
            {t("settings.own")}
          </h3>
          {instructions.data?.text ? (
            <Markdown text={instructions.data.text} copyLabel={t("copy")} copiedLabel={t("copied")} className="text-ui" />
          ) : (
            <p className="text-ui text-fg-muted">{t("settings.noOwn")}</p>
          )}
        </section>
        <details className="rounded-md border border-border p-3">
          <summary className="cursor-pointer font-semibold">{t("create.effective")}</summary>
          <Markdown text={instructions.data?.effective ?? ""} copyLabel={t("copy")} copiedLabel={t("copied")} className="mt-2 text-ui" />
        </details>
      </div>
    ),
    veszelyes: (
      <div className="flex flex-col gap-3">
        <h3 className="text-title font-semibold text-danger">{t("settings.dangerTitle")}</h3>
        {actor.state === "paused" ? (
          <Button onClick={() => setState.mutate({ id: actor.id, state: "active" })}>{t("settings.resume")}</Button>
        ) : (
          <Button disabled={archived || actor.state === "stopped"} onClick={() => setState.mutate({ id: actor.id, state: "paused" })}>
            {t("settings.pause")}
          </Button>
        )}
        <Button variant="danger" disabled={archived || actor.state === "stopped"} onClick={() => setDanger("stop")}>
          {t("settings.stop")}
        </Button>
        <Button variant="danger" icon={<Trash2 className="size-4" />} disabled={archived} onClick={() => setDanger("delete")}>
          {t("settings.delete")}
        </Button>
      </div>
    ),
  };

  const footer =
    tab === "profil" && !readOnly ? (
      <Button variant="primary" disabled={!dirty} loading={updateProfile.isPending} onClick={saveProfile}>
        {t("settings.save")}
      </Button>
    ) : tab === "keretek" && !readOnly && config ? (
      <Button
        variant="primary"
        disabled={!limits}
        loading={setLimitsM.isPending}
        onClick={() =>
          setLimitsM.mutate(
            { agentId: actor.id, max_session_tokens: curLimits.session, daily_token_limit: curLimits.daily },
            { onSuccess: () => (setLimits(null), toast(t("settings.saved"))), onError: (e) => toast(e.message, { tone: "danger" }) },
          )
        }
      >
        {t("settings.save")}
      </Button>
    ) : undefined;

  return (
    <>
      <LargeDialog
        open
        onOpenChange={(o) => !o && guard({ close: true })}
        title={t("settings.title")}
        header={header}
        tabs={settingsTabs.filter((id) => admin || id !== "veszelyes").map((id) => ({ id, label: t(`settings.tabs.${id}`), marker: id === "konfiguracio" || id === "utasitas" ? <Lock aria-hidden className="size-3" /> : undefined }))}
        active={tab}
        onSelect={(id) => guard({ next: id as SettingsTab })}
        navLabel={t("settings.tabsLabel")}
        banner={
          <div className="flex flex-col gap-2">
            {archived ? <Notice>{t("settings.archivedNotice")}</Notice> : null}
            {!admin && !archived ? <Notice>{t("settings.adminOnly")}</Notice> : null}
            {changedElsewhere && !archived ? (
              <Notice
                tone="warning"
                action={
                  <Button
                    size="sm"
                    onClick={() => {
                      seen.current = savedKey;
                      setChangedElsewhere(false);
                      if (!dirty) setProfile(saved);
                    }}
                  >
                    {t("settings.refresh")}
                  </Button>
                }
              >
                {t("settings.changedElsewhere")}
              </Notice>
            ) : null}
          </div>
        }
        footer={footer}
        closeLabel={t("close")}
        backLabel={t("back")}
      >
        {body[tab]}
      </LargeDialog>
      <Dialog
        open={ask !== null}
        onOpenChange={(o) => !o && setAsk(null)}
        title={t("settings.unsaved")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setAsk(null)}>{t("create.keepEditing")}</Button>
            <Button
              variant="danger"
              onClick={() => {
                const next = ask;
                setAsk(null);
                setProfile(saved);
                if (next?.close) onClose();
                else if (next?.next) onTab(next.next);
              }}
            >
              {t("create.discard")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
      <Dialog
        open={danger === "stop"}
        onOpenChange={(o) => !o && setDanger(null)}
        title={t("settings.stopTitle", { handle: actor.handle })}
        description={t("settings.stopBody")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setDanger(null)}>{t("cancel")}</Button>
            <Button variant="danger" onClick={() => setState.mutate({ id: actor.id, state: "stopped" }, { onSuccess: () => setDanger(null) })}>
              {t("settings.stop")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
      <Dialog
        open={danger === "delete"}
        onOpenChange={(o) => !o && (setDanger(null), setTyped(""))}
        title={t("settings.deleteTitle", { handle: actor.handle })}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setDanger(null)}>{t("cancel")}</Button>
            <Button
              variant="danger"
              disabled={typed !== actor.handle}
              loading={archive.isPending}
              onClick={() =>
                archive.mutate(actor.id, {
                  onSuccess: () => {
                    toast(t("settings.deleted", { handle: actor.handle }));
                    setDanger(null);
                    onClose();
                  },
                  onError: (e) => toast(e.message, { tone: "danger" }),
                })
              }
            >
              {t("settings.delete")}
            </Button>
          </>
        }
      >
        <ul className="mb-3 list-disc pl-5 text-ui">
          {(["runs", "terminals", "access", "messages", "handle"] as const).map((k) => (
            <li key={k}>{t(`settings.deleteList.${k}`)}</li>
          ))}
        </ul>
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("settings.typeHandle", { handle: actor.handle })}</span>
          <Input value={typed} onChange={(e) => setTyped(e.target.value)} className="font-mono" autoComplete="off" />
        </label>
      </Dialog>
    </>
  );
}
