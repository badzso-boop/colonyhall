// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Copy, FileText, FolderOpen, KeyRound, Lock, ShieldAlert, Unlock } from "lucide-react";
import { FolderPicker, useHosts, type Host, type Selection } from "@/features/hosts";
import { useActors } from "@/features/identity";
import { ApiError } from "@/shared/api";
import { cn, hueVar, hues } from "@/shared/lib";
import { Button, Checkbox, Dialog, Input, LargeDialog, Markdown, Notice, Select, Switch, Textarea, toast } from "@/shared/ui";
import { useAgentActors, useCreateAgent, useJoinableChannels, useSharedRules } from "../api";
import {
  MaxInstructions,
  canBeDocsOnly,
  emptyDraft,
  firstInvalidStep,
  knownModels,
  looksSecret,
  partnerCandidates,
  permissionModes,
  riskyModes,
  sessionPresets,
  stepErrors,
  stepForError,
  steps,
  toRequest,
  type Draft,
  type HandleInfo,
  type Step,
} from "../creation";
import { hostOnline } from "../directory";

const placeholders = ["handle", "admins", "workspace"] as const;

/** The shared rules then the own instructions, placeholders filled for the new handle (client-side preview). */
function effectivePreview(shared: string, own: string, handle: string): string {
  const fill = (s: string) => s.replaceAll("{handle}", handle || "…").replaceAll("{workspace}", "Colonyhall");
  return own.trim() ? `${fill(shared)}\n\n## Instructions for @${handle || "…"}\n\n${fill(own)}` : fill(shared);
}

function Field({ label, lock, children, hint, error }: { label: string; lock?: "fixed" | "mutable"; children: ReactNode; hint?: string; error?: string }) {
  const { t } = useTranslation("agents");
  return (
    <label className="flex flex-col gap-1">
      <span className="flex items-center gap-1.5 text-meta font-semibold text-fg-muted">
        {lock === "fixed" ? <Lock aria-label={t("create.fixed")} className="size-3" /> : null}
        {label}
        {lock === "mutable" ? <span className="font-normal text-fg-subtle">· {t("create.mutable")}</span> : null}
      </span>
      {children}
      {hint ? <span className="text-meta text-fg-subtle">{hint}</span> : null}
      {error ? <span className="text-meta text-danger">{error}</span> : null}
    </label>
  );
}

function Choice({ selected, onSelect, title, body, icon }: { selected: boolean; onSelect: () => void; title: string; body: string; icon?: ReactNode }) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onSelect}
      className={cn("flex flex-1 flex-col gap-1 rounded-lg border p-3 text-left", selected ? "border-accent bg-accent-soft" : "border-border hover:bg-surface-sunken")}
    >
      <span className="flex items-center gap-2 font-semibold">
        {icon}
        {title}
      </span>
      <span className="text-meta text-fg-muted">{body}</span>
    </button>
  );
}

export function ColorSwatches({ value, onChange, inherit }: { value: string; onChange: (c: string) => void; inherit?: string }) {
  const { t } = useTranslation("agents");
  return (
    <div role="radiogroup" aria-label={t("create.color")} className="flex flex-wrap items-center gap-1.5">
      {inherit !== undefined ? (
        <button
          type="button"
          role="radio"
          aria-checked={value === ""}
          onClick={() => onChange("")}
          className={cn("h-6 rounded-full border px-2 text-meta", value === "" ? "border-fg font-semibold" : "border-border text-fg-muted")}
        >
          {t("create.hostColor")}
        </button>
      ) : null}
      {hues.map((h) => (
        <button
          key={h}
          type="button"
          role="radio"
          aria-checked={value === h}
          aria-label={t(`hue.${h}`)}
          title={t(`hue.${h}`)}
          onClick={() => onChange(h)}
          className="size-6 rounded-full ring-offset-2 ring-offset-surface-raised aria-checked:ring-2 aria-checked:ring-fg"
          style={{ background: hueVar(h) }}
        />
      ))}
    </div>
  );
}

/**
 * "Új agent" (round 33, part 4): every setting of an agent, in six
 * steps, because none of the configuration can change afterwards.
 */
export function CreateAgentDialog({ initial, onClose }: { initial?: Draft; onClose: () => void }) {
  const { t } = useTranslation("agents");
  const [d, setD] = useState<Draft>(() => initial ?? emptyDraft());
  const [step, setStep] = useState<Step>("profile");
  const [visited, setVisited] = useState<Set<Step>>(new Set(["profile"]));
  const [ack, setAck] = useState(false);
  const [picker, setPicker] = useState<null | "root" | "worktree" | "grants">(null);
  const [confirmClose, setConfirmClose] = useState(false);
  const [token, setToken] = useState<string | null>(null);
  const [serverError, setServerError] = useState<{ step: Step; message: string } | null>(null);
  const hosts = useHosts();
  const actors = useAgentActors(true);
  const channels = useJoinableChannels(true);
  const rules = useSharedRules(step === "instructions" || step === "summary");
  const create = useCreateAgent();

  const handles: HandleInfo = useMemo(() => {
    const active = new Set<string>();
    const archived = new Set<string>();
    for (const a of actors.data ?? []) (a.archived_at ? archived : active).add(a.handle);
    return { active, archived };
  }, [actors.data]);
  const host: Host | undefined = hosts.data?.find((h) => h.id === d.hostId);
  const partners = useMemo(() => partnerCandidates(hosts.data), [hosts.data]);
  const partner = partners.find((p) => p.id === d.partnerId);
  const everyone = useActors();
  const people = useMemo(() => [...(everyone.data?.values() ?? [])].filter((a) => a.kind === "human" && !a.archived_at), [everyone.data]);
  const set = (patch: Partial<Draft>) => setD((x) => ({ ...x, ...patch }));
  const errs = stepErrors(d, step, handles);
  const go = (s: Step) => {
    setStep(s);
    setVisited((v) => new Set(v).add(s));
  };
  const dirty = JSON.stringify({ ...d, grants: [...d.grants] }) !== JSON.stringify({ ...(initial ?? emptyDraft()), grants: [...(initial ?? emptyDraft()).grants] });

  const submit = () => {
    const bad = firstInvalidStep(d, handles);
    if (bad) {
      go(bad);
      return;
    }
    setServerError(null);
    create.mutate(toRequest(d), {
      onSuccess: (res) => {
        toast(t("create.created", { handle: d.handle }));
        if (res.token) setToken(res.token);
        else onClose();
      },
      onError: (e) => {
        const message = e instanceof ApiError ? e.message : String(e);
        const s = e instanceof ApiError && e.status === 409 ? "profile" : stepForError(message);
        setServerError({ step: s, message });
        go(s);
      },
    });
  };

  if (token)
    return (
      <Dialog open onOpenChange={() => onClose()} title={t("create.tokenTitle")} description={t("create.tokenBody")} closeLabel={t("close")}>
        <div className="flex items-center gap-2">
          <code className="min-w-0 flex-1 truncate rounded-md bg-surface-sunken px-2 py-1 font-mono text-meta">{token}</code>
          <Button size="sm" icon={<Copy className="size-3.5" />} onClick={() => void navigator.clipboard?.writeText(token).then(() => toast(t("copied")))}>
            {t("copy")}
          </Button>
        </div>
      </Dialog>
    );

  const fixedBanner = (
    <Notice tone="warning" icon={<Lock className="size-4" />}>
      {t("create.banner")}
    </Notice>
  );
  const clientList = host?.report.clients ?? [];
  const tabs = steps.map((s, i) => ({
    id: s,
    label: t(`create.steps.${s}`),
    marker: String(i + 1),
    invalid: (visited.has(s) && s !== step && Object.keys(stepErrors(d, s, handles)).length > 0) || serverError?.step === s,
  }));
  const idx = steps.indexOf(step);

  const body: Record<Step, ReactNode> = {
    profile: (
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          label={t("create.handle")}
          lock="fixed"
          error={errs.handle ? t(errs.handle) : undefined}
          hint={handles.archived.has(d.handle) && !handles.active.has(d.handle) ? t("create.handleArchived") : t("create.handleHint")}
        >
          <Input value={d.handle} onChange={(e) => set({ handle: e.target.value.toLowerCase() })} className="font-mono" maxLength={32} autoFocus />
        </Field>
        <Field label={t("create.displayName")} lock="mutable">
          <Input value={d.displayName} onChange={(e) => set({ displayName: e.target.value })} maxLength={80} />
        </Field>
        <Field label={t("create.emoji")} lock="mutable">
          <Input value={d.avatarEmoji} onChange={(e) => set({ avatarEmoji: e.target.value })} maxLength={8} className="w-24" />
        </Field>
        <Field label={t("create.color")} lock="mutable">
          <ColorSwatches value={d.color} onChange={(color) => set({ color })} inherit="" />
        </Field>
        <Field label={t("create.owner")} lock="mutable" hint={t("create.ownerHint")}>
          <Select value={d.ownerId} onChange={(e) => set({ ownerId: e.target.value })}>
            <option value="">{t("create.ownerNobody")}</option>
            {people.map((p) => (
              <option key={p.id} value={p.id}>
                {p.display_name} (@{p.handle})
              </option>
            ))}
          </Select>
        </Field>
        <div className="sm:col-span-2">
          <Field label={t("create.description")} lock="mutable" error={errs.description ? t(errs.description) : undefined}>
            <Textarea rows={2} value={d.description} onChange={(e) => set({ description: e.target.value })} maxLength={300} />
          </Field>
        </div>
      </div>
    ),
    machine: (
      <div className="flex flex-col gap-4">
        <div role="radiogroup" aria-label={t("create.host")} className="grid gap-2 sm:grid-cols-2">
          {(hosts.data ?? [])
            .filter((h) => h.state === "approved")
            .map((h) => (
              <Choice
                key={h.id}
                selected={!d.apiKey && d.hostId === h.id}
                onSelect={() => set({ apiKey: false, hostId: h.id, grants: h.id === d.hostId ? d.grants : new Map(), rootDir: h.id === d.hostId ? d.rootDir : "", worktreeRepo: h.id === d.hostId ? d.worktreeRepo : "" })}
                icon={<span aria-hidden className="size-3 rounded-full" style={{ background: hueVar(h.color) }} />}
                title={h.name}
                body={`${hostOnline(h, new Date()) ? t("create.online") : t("create.offline")} · ${h.report.clients.map((c) => `${c.name} ${c.version ?? ""}`.trim()).join(", ") || t("create.noClients")}`}
              />
            ))}
          <Choice selected={d.apiKey} onSelect={() => set({ apiKey: true })} icon={<KeyRound className="size-4" />} title={t("create.apiKey")} body={t("create.apiKeyBody")} />
        </div>
        {errs.hostId ? <p className="text-meta text-danger">{t(errs.hostId)}</p> : null}
        {!d.apiKey && host ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label={t("create.client")} lock="fixed">
              <Select value={d.client} onChange={(e) => set({ client: e.target.value as Draft["client"], permissionMode: "auto", approvals: "none", docsOnly: false })}>
                {(["claude-code", "antigravity"] as const).map((c) => (
                  <option key={c} value={c} disabled={!clientList.some((x) => x.name === c)}>
                    {c}
                    {clientList.some((x) => x.name === c) ? "" : ` (${t("create.notReported")})`}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label={t("create.model")} lock="fixed">
              <Input list="colony-models" value={d.model} onChange={(e) => set({ model: e.target.value })} placeholder="sonnet" />
            </Field>
            <Field label={t("create.workModel")} lock="fixed" hint={t("create.workModelHint")}>
              <Input list="colony-models" value={d.workModel} onChange={(e) => set({ workModel: e.target.value })} />
            </Field>
            <datalist id="colony-models">
              {knownModels[d.client].map((m) => (
                <option key={m} value={m} />
              ))}
            </datalist>
            <Field label={t("create.mode")} lock="fixed" hint={t(`create.modeHint.${d.mode}`)}>
              <Select value={d.mode} onChange={(e) => set({ mode: e.target.value as Draft["mode"], sandbox: e.target.value === "compact", docsOnly: false })}>
                <option value="direct">direct</option>
                <option value="compact">compact</option>
              </Select>
            </Field>
            <Field label={t("create.permission")} lock="fixed" hint={riskyModes.has(d.permissionMode) ? t("create.riskyMode") : t("create.permissionHint")}>
              <Select value={d.permissionMode} onChange={(e) => set({ permissionMode: e.target.value })}>
                {permissionModes[d.client].map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </Select>
            </Field>
            <label className="flex items-center gap-2 self-end text-ui">
              <Switch checked={d.allowWork} onCheckedChange={(v) => set({ allowWork: v })} />
              <Lock aria-hidden className="size-3 text-fg-muted" />
              {t("create.allowWork")}
            </label>
          </div>
        ) : null}
      </div>
    ),
    access: d.apiKey ? (
      <Notice>{t("create.apiKeyNoAccess")}</Notice>
    ) : (
      <div className="flex flex-col gap-4">
        <div role="radiogroup" aria-label={t("create.access")} className="flex gap-2 max-sm:flex-col">
          <Choice selected={d.sandbox} onSelect={() => set({ sandbox: true, allowPreview: false, docsOnly: false })} icon={<Lock className="size-4" />} title={t("create.sandbox")} body={t("create.sandboxBody")} />
          <Choice
            selected={!d.sandbox}
            onSelect={() => set({ sandbox: false, allowWebRead: false })}
            icon={<ShieldAlert className="size-4 text-warning" />}
            title={t("create.fullAccess")}
            body={t("create.fullAccessBody")}
          />
        </div>
        {canBeDocsOnly(d) || d.docsOnly ? (
          <div className="flex flex-col gap-2 rounded-md border border-border p-3">
            <label className="flex items-center gap-2 text-ui">
              <Switch checked={d.docsOnly} onCheckedChange={(v) => set(v ? { docsOnly: true, approvals: "none", allowPreview: false } : { docsOnly: false, partnerId: "" })} />
              <Lock aria-hidden className="size-3 text-fg-muted" />
              <FileText aria-hidden className="size-4 text-fg-muted" />
              {t("create.docsOnly")}
            </label>
            <span className="text-meta text-fg-subtle">{t("create.docsOnlyHint")}</span>
            {errs.docsOnly ? <span className="text-meta text-danger">{t(errs.docsOnly)}</span> : null}
            {d.docsOnly ? (
              <Field
                label={t("create.partner")}
                lock="fixed"
                hint={partners.length ? t("create.partnerHint") : t("create.noPartners")}
                error={errs.partnerId ? t(errs.partnerId) : undefined}
              >
                <Select value={d.partnerId} onChange={(e) => set({ partnerId: e.target.value })}>
                  <option value="">{t("create.choose")}</option>
                  {partners.map((p) => (
                    <option key={p.id} value={p.id}>
                      @{p.handle} · {p.host}
                    </option>
                  ))}
                </Select>
              </Field>
            ) : null}
          </div>
        ) : null}
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("create.rootDir")} lock="fixed" error={errs.rootDir ? t(errs.rootDir) : undefined}>
            <Button size="sm" icon={<FolderOpen className="size-3.5" />} onClick={() => setPicker("root")}>
              <span className="truncate font-mono">{d.rootDir || t("create.choose")}</span>
            </Button>
          </Field>
          <Field label={t("create.worktree")} lock="fixed" hint={t("create.worktreeHint")} error={errs.worktreeRepo ? t(errs.worktreeRepo) : undefined}>
            <Button size="sm" icon={<FolderOpen className="size-3.5" />} onClick={() => setPicker("worktree")}>
              <span className="truncate font-mono">{d.worktreeRepo || t("create.none")}</span>
            </Button>
          </Field>
        </div>
        <Field label={t("create.grants")} lock="fixed" hint={d.sandbox ? t("create.grantsHint") : t("create.grantsFullHint")}>
          <div className="flex flex-col gap-1">
            {[...d.grants].map(([p, a]) => (
              <span key={p} className="font-mono text-meta">
                {a === "write" ? "✎" : "👁"} {p} · {t(`create.${a}`)}
              </span>
            ))}
            <Button size="sm" className="self-start" icon={<FolderOpen className="size-3.5" />} onClick={() => setPicker("grants")}>
              {t("create.chooseGrants")}
            </Button>
          </div>
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("create.approvals")} lock="fixed" error={errs.approvals ? t(errs.approvals) : undefined} hint={t("create.approvalsHint")}>
            <Select value={d.approvals} onChange={(e) => set({ approvals: e.target.value as Draft["approvals"] })}>
              <option value="none">none</option>
              <option value="chat" disabled={d.docsOnly}>
                chat
              </option>
            </Select>
          </Field>
          <div className="flex flex-col gap-2">
            <label className="flex items-center gap-2 text-ui">
              <Switch checked={d.allowPreview} disabled={d.sandbox || d.mode !== "direct" || d.docsOnly} onCheckedChange={(v) => set({ allowPreview: v })} />
              <Lock aria-hidden className="size-3 text-fg-muted" />
              {t("create.preview")}
            </label>
            <span className="text-meta text-fg-subtle">{t("create.previewHint")}</span>
            <label className="flex items-center gap-2 text-ui">
              <Switch checked={d.allowWebRead} disabled={!d.sandbox} onCheckedChange={(v) => set({ allowWebRead: v })} />
              <Lock aria-hidden className="size-3 text-fg-muted" />
              {t("create.webRead")}
            </label>
            <span className="text-meta text-fg-subtle">{t("create.webReadHint")}</span>
          </div>
        </div>
        {host ? (
          <Notice>
            {t("create.guardOnly", {
              sandbox: host.report.sandbox || t("no"),
              unsandboxed: host.report.allow_unsandboxed ? t("yes") : t("no"),
            })}
          </Notice>
        ) : null}
      </div>
    ),
    instructions: (
      <div className="flex flex-col gap-2">
        <div role="toolbar" aria-label={t("create.insert")} className="flex flex-wrap items-center gap-1">
          <Lock aria-hidden className="size-3 text-fg-muted" />
          <span className="text-meta text-fg-subtle">{t("create.insert")}:</span>
          {placeholders.map((p) => (
            <button key={p} type="button" onClick={() => set({ instructions: `${d.instructions}{${p}}` })} className="rounded-sm bg-info-soft px-1.5 py-0.5 font-mono text-meta text-info">
              {`{${p}}`}
            </button>
          ))}
          <span className={cn("ml-auto text-meta tabular-nums", [...d.instructions].length > MaxInstructions * 0.9 && "font-semibold text-warning")}>
            {t("create.count", { n: [...d.instructions].length, max: MaxInstructions })}
          </span>
        </div>
        <Notice tone={looksSecret(d.instructions) ? "danger" : "warning"}>{t("create.secretWarning")}</Notice>
        {d.docsOnly ? <Notice icon={<FileText className="size-4" />}>{t("create.docsOnlyBlock", { partner: partner?.handle ?? "…" })}</Notice> : null}
        <div className="grid gap-3 md:grid-cols-2">
          <Textarea aria-label={t("create.instructions")} rows={14} value={d.instructions} onChange={(e) => set({ instructions: e.target.value })} className="font-mono text-ui" />
          <section aria-label={t("create.effective")} className="max-h-96 overflow-y-auto rounded-md border border-border p-3">
            <p className="mb-2 text-meta font-semibold text-fg-muted">{t("create.effective")}</p>
            <Markdown text={effectivePreview(rules.data?.text ?? "", d.instructions, d.handle)} copyLabel={t("copy")} copiedLabel={t("copied")} className="text-ui" />
          </section>
        </div>
        {errs.instructions ? <p className="text-meta text-danger">{t(errs.instructions)}</p> : null}
      </div>
    ),
    limits: (
      <div className="flex flex-col gap-4">
        <Field label={t("create.sessionLimit")} lock="mutable" hint={t("create.sessionHint")}>
          <div role="radiogroup" className="flex flex-wrap gap-1">
            {sessionPresets.map((n) => (
              <button
                key={n}
                type="button"
                role="radio"
                aria-checked={d.maxSessionTokens === n}
                onClick={() => set({ maxSessionTokens: n })}
                className={cn("rounded-md border px-2.5 py-1 text-ui", d.maxSessionTokens === n ? "border-accent bg-accent-soft font-semibold" : "border-border")}
              >
                {n ? `${n / 1000}k` : t("create.noLimit")}
              </button>
            ))}
          </div>
        </Field>
        <Field label={t("create.dailyLimit")} lock="mutable" hint={t("create.dailyHint")} error={errs.limits ? t(errs.limits) : undefined}>
          <Input type="number" min={0} value={d.dailyTokenLimit} onChange={(e) => set({ dailyTokenLimit: Number(e.target.value) })} className="w-48" />
        </Field>
        <Field label={t("create.channels")} lock="mutable" hint={t("create.channelsHint")}>
          <div className="flex max-h-48 flex-col gap-1 overflow-y-auto rounded-md border border-border p-2">
            {(channels.data ?? []).map((c) => (
              <label key={c.id} className="flex items-center gap-2 text-ui">
                <Checkbox
                  checked={d.channelIds.includes(c.id)}
                  onCheckedChange={(v) => set({ channelIds: v ? [...d.channelIds, c.id] : d.channelIds.filter((x) => x !== c.id) })}
                />
                #{c.name}
              </label>
            ))}
          </div>
        </Field>
      </div>
    ),
    summary: (
      <div className="flex flex-col gap-4">
        <div className="grid gap-3 md:grid-cols-2">
          <section aria-label={t("create.fixedTitle")} className="rounded-md border border-border p-3">
            <h3 className="mb-2 flex items-center gap-1.5 font-semibold">
              <Lock aria-hidden className="size-4" />
              {t("create.fixedTitle")}
            </h3>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-ui">
              <dt className="text-fg-muted">{t("create.handle")}</dt>
              <dd className="font-mono">@{d.handle}</dd>
              {d.apiKey ? (
                <>
                  <dt className="text-fg-muted">{t("create.host")}</dt>
                  <dd>{t("create.apiKey")}</dd>
                </>
              ) : (
                <>
                  <dt className="text-fg-muted">{t("create.host")}</dt>
                  <dd>{host?.name ?? "–"}</dd>
                  <dt className="text-fg-muted">{t("create.client")}</dt>
                  <dd>
                    {d.client} · {d.model || "–"} · {d.mode}
                  </dd>
                  <dt className="text-fg-muted">{t("create.access")}</dt>
                  <dd>{d.sandbox ? t("create.sandbox") : t("create.fullAccess")}</dd>
                  {d.docsOnly ? (
                    <>
                      <dt className="text-fg-muted">{t("create.writeScope")}</dt>
                      <dd>{t("create.docsOnlySummary", { partner: partner?.handle ?? "–" })}</dd>
                    </>
                  ) : null}
                  <dt className="text-fg-muted">{t("create.permission")}</dt>
                  <dd>{d.permissionMode}</dd>
                  <dt className="text-fg-muted">{t("create.rootDir")}</dt>
                  <dd className="font-mono text-meta">{d.rootDir || d.worktreeRepo || "–"}</dd>
                  <dt className="text-fg-muted">{t("create.grants")}</dt>
                  <dd className="font-mono text-meta">{[...d.grants].map(([p, a]) => `${p} (${t(`create.${a}`)})`).join(", ") || "–"}</dd>
                </>
              )}
              <dt className="text-fg-muted">{t("create.instructions")}</dt>
              <dd className="line-clamp-3 text-meta">{d.instructions.trim() || "–"}</dd>
            </dl>
          </section>
          <section aria-label={t("create.mutableTitle")} className="rounded-md border border-border p-3">
            <h3 className="mb-2 flex items-center gap-1.5 font-semibold">
              <Unlock aria-hidden className="size-4" />
              {t("create.mutableTitle")}
            </h3>
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-ui">
              <dt className="text-fg-muted">{t("create.displayName")}</dt>
              <dd>{d.displayName || d.handle}</dd>
              <dt className="text-fg-muted">{t("create.color")}</dt>
              <dd>{d.color ? t(`hue.${d.color}`) : t("create.hostColor")}</dd>
              <dt className="text-fg-muted">{t("create.sessionLimit")}</dt>
              <dd>{d.maxSessionTokens || t("create.noLimit")}</dd>
              <dt className="text-fg-muted">{t("create.dailyLimit")}</dt>
              <dd>{d.dailyTokenLimit || t("create.noLimit")}</dd>
              <dt className="text-fg-muted">{t("create.channels")}</dt>
              <dd>{(channels.data ?? []).filter((c) => d.channelIds.includes(c.id)).map((c) => `#${c.name}`).join(", ") || "–"}</dd>
            </dl>
          </section>
        </div>
        <label className="flex items-start gap-2 text-ui">
          <Checkbox checked={ack} onCheckedChange={(v) => setAck(v === true)} aria-label={t("create.ack")} className="mt-0.5" />
          <span>{t("create.ack")}</span>
        </label>
      </div>
    ),
  };

  return (
    <>
      <LargeDialog
        open
        onOpenChange={(o) => !o && onClose()}
        beforeClose={() => {
          if (!dirty) return true;
          setConfirmClose(true);
          return false;
        }}
        title={t("create.title")}
        tabs={tabs}
        active={step}
        onSelect={(s) => go(s as Step)}
        navLabel={t("create.stepsLabel")}
        banner={
          <div className="flex flex-col gap-2">
            {fixedBanner}
            {serverError && serverError.step === step ? <Notice tone="danger">{serverError.message}</Notice> : null}
          </div>
        }
        closeLabel={t("close")}
        backLabel={t("back")}
        footer={
          <>
            <Button variant="ghost" disabled={idx === 0} onClick={() => go(steps[idx - 1]!)}>
              {t("create.prev")}
            </Button>
            {step === "summary" ? (
              <Button variant="primary" disabled={!ack} loading={create.isPending} onClick={submit}>
                {t("create.submit")}
              </Button>
            ) : (
              <Button variant="primary" onClick={() => go(steps[idx + 1]!)}>
                {t("create.next")}
              </Button>
            )}
          </>
        }
      >
        {body[step]}
      </LargeDialog>
      {picker && host ? (
        <FolderPicker
          hostId={host.id}
          hostLastSeen={host.last_seen_at}
          open
          onOpenChange={(o) => !o && setPicker(null)}
          mode={picker === "grants" ? "multi" : "single"}
          gitOnly={picker === "worktree"}
          allowTyped
          initial={picker === "grants" ? d.grants : picker === "root" ? d.rootDir : d.worktreeRepo}
          onConfirm={(sel: Selection) => {
            if (picker === "grants") set({ grants: new Map(sel) });
            else if (picker === "root") set({ rootDir: [...sel.keys()][0] ?? "" });
            else set({ worktreeRepo: [...sel.keys()][0] ?? "" });
          }}
        />
      ) : null}
      <Dialog
        open={confirmClose}
        onOpenChange={setConfirmClose}
        title={t("create.discardTitle")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirmClose(false)}>{t("create.keepEditing")}</Button>
            <Button variant="danger" onClick={onClose}>
              {t("create.discard")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </>
  );
}
