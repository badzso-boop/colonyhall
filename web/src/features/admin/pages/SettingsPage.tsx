// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";
import { CircleCheck, CircleMinus, Settings, Trash2 } from "lucide-react";
import { useChannels } from "@/features/conversation";
import { useActors } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { cn, fullTime } from "@/shared/lib";
import { Button, Dialog, EmptyState, Input, Notice, Select, Skeleton, Tab, TabList, Tabs, toast } from "@/shared/ui";
import {
  useActiveGrants,
  useAddGitMapping,
  useAddPrice,
  useCompactionSettings,
  useCurrency,
  useDelegationSettings,
  useFileFetchSettings,
  useGitMappings,
  usePrices,
  useRemoveGitMapping,
  useRevokeGrant,
  useSetCompactionSettings,
  useSetCurrency,
  useSetDelegationSettings,
  useSetFileFetchSettings,
  useSystemStatus,
} from "../api";
import { AdminPage } from "../components/AdminPage";
import { priceFields, priceHistory, validRepo, type PriceField } from "../model";
import { StorageCard } from "../components/StorageCard";

const sections = ["tomorites", "fajllekeres", "feladatatadas", "gitea", "arak", "fajlok"] as const;
type Section = (typeof sections)[number];

/** Beállítások (/admin/settings): a left section list, tabs on a phone; each section saves on its own. */
export default function SettingsPage() {
  const { t } = useTranslation("admin");
  const [params, setParams] = useSearchParams();
  const raw = params.get("section");
  const active: Section = sections.includes(raw as Section) ? (raw as Section) : "tomorites";
  const select = (s: string) => setParams({ section: s }, { replace: true });
  return (
    <AdminPage icon={<Settings className="size-5" />} title={t("settings.title")}>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden md:flex-row">
        <nav aria-label={t("settings.sections")} className="hidden w-52 shrink-0 flex-col gap-px border-r border-border p-2 md:flex">
          {sections.map((s) => (
            <button
              key={s}
              type="button"
              aria-current={s === active ? "page" : undefined}
              onClick={() => select(s)}
              className={cn("h-8 rounded-md px-2 text-left text-ui text-fg-muted hover:bg-surface-sunken", s === active && "bg-accent-soft font-semibold text-fg")}
            >
              {t(`settings.${s}.title`)}
            </button>
          ))}
        </nav>
        <div className="shrink-0 overflow-x-auto px-2 pt-2 md:hidden">
          <Tabs value={active} onValueChange={select}>
            <TabList label={t("settings.sections")}>
              {sections.map((s) => (
                <Tab key={s} value={s}>
                  <span className="whitespace-nowrap">{t(`settings.${s}.title`)}</span>
                </Tab>
              ))}
            </TabList>
          </Tabs>
        </div>
        <div className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4">
          <div className="mx-auto flex max-w-3xl flex-col gap-4">
            {active === "tomorites" ? <CompactionSection /> : null}
            {active === "fajllekeres" ? <FileFetchSection /> : null}
            {active === "feladatatadas" ? <DelegationSection /> : null}
            {active === "gitea" ? <GiteaSection /> : null}
            {active === "arak" ? <PricesSection /> : null}
            {active === "fajlok" ? <FilesSection /> : null}
          </div>
        </div>
      </div>
    </AdminPage>
  );
}

function Save({ changes, disabled, loading, onSave, children }: { changes: string[]; disabled?: boolean; loading?: boolean; onSave: () => void; children: ReactNode }) {
  const { t } = useTranslation("admin");
  return (
    <div className="flex flex-col items-start gap-2 border-t border-border pt-3">
      {changes.length ? (
        <div role="status" className="text-ui">
          <p className="font-semibold">{t("settings.whatChanges")}</p>
          <ul className="tabular list-disc pl-5 text-fg-muted">
            {changes.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ul>
        </div>
      ) : null}
      <Button variant="primary" disabled={disabled || !changes.length} loading={loading} onClick={onSave}>
        {children}
      </Button>
    </div>
  );
}

function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1 text-ui">
      <span className="font-medium">{label}</span>
      {children}
      {hint ? <span className="text-meta text-fg-subtle">{hint}</span> : null}
    </label>
  );
}

const thresholds = [150_000, 350_000, 500_000, 1_000_000];

function CompactionSection() {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const settings = useCompactionSettings();
  const actors = useActors();
  const save = useSetCompactionSettings();
  const [threshold, setThreshold] = useState<number | null>(null);
  const [scribe, setScribe] = useState<string | null>(null);
  if (settings.isPending) return <Skeleton className="h-32 w-full" />;
  if (settings.isError) return <p className="text-danger">{settings.error.message}</p>;
  const current = settings.data;
  const value = threshold ?? current.compact_threshold_tokens;
  const agent = scribe ?? current.compactor_agent_id ?? "";
  const handle = (id: string | null | undefined) => (id ? `@${actors.data?.get(id)?.handle ?? "?"}` : t("settings.tomorites.noScribe"));
  const changes = [
    value !== current.compact_threshold_tokens ? t("settings.tomorites.changeThreshold", { from: current.compact_threshold_tokens.toLocaleString(locale), to: value.toLocaleString(locale) }) : null,
    agent !== (current.compactor_agent_id ?? "") ? t("settings.tomorites.changeScribe", { from: handle(current.compactor_agent_id), to: handle(agent) }) : null,
  ].filter((c): c is string => c !== null);
  const valid = value >= 20_000 && value <= 2_000_000;
  return (
    <section className="flex flex-col gap-3">
      <p className="text-ui text-fg-muted">{t("settings.tomorites.body")}</p>
      <div className="flex flex-wrap gap-2" role="group" aria-label={t("settings.tomorites.presets")}>
        {thresholds.map((n) => (
          <Button key={n} size="sm" variant={value === n ? "primary" : "secondary"} onClick={() => setThreshold(n)}>
            {n.toLocaleString(locale)}
          </Button>
        ))}
      </div>
      <Field label={t("settings.tomorites.threshold")} hint={t("settings.tomorites.thresholdHint")}>
        <Input type="number" min={20000} max={2000000} step={10000} value={value} onChange={(e) => setThreshold(Number(e.target.value))} className="w-44" />
      </Field>
      <Field label={t("settings.tomorites.scribe")}>
        <Select value={agent} onChange={(e) => setScribe(e.target.value)} className="w-64 max-w-full">
          <option value="">{t("settings.tomorites.noScribe")}</option>
          {[...(actors.data?.values() ?? [])]
            .filter((a) => a.kind === "agent" && !a.archived_at)
            .map((a) => (
              <option key={a.id} value={a.id}>
                @{a.handle}
              </option>
            ))}
        </Select>
      </Field>
      <Save
        changes={changes}
        disabled={!valid}
        loading={save.isPending}
        onSave={() =>
          save.mutate(
            { compact_threshold_tokens: value, compactor_agent_id: agent || null },
            {
              onSuccess: () => {
                setThreshold(null);
                setScribe(null);
                toast(t("settings.saved"));
              },
              onError: (e) => toast(e.message, { tone: "danger" }),
            },
          )
        }
      >
        {t("settings.save")}
      </Save>
    </section>
  );
}

function FileFetchSection() {
  const { t } = useTranslation("admin");
  const settings = useFileFetchSettings();
  const save = useSetFileFetchSettings();
  const [image, setImage] = useState<number | null>(null);
  const [file, setFile] = useState<number | null>(null);
  if (settings.isPending) return <Skeleton className="h-32 w-full" />;
  if (settings.isError) return <p className="text-danger">{settings.error.message}</p>;
  const img = image ?? settings.data.max_image_mb;
  const fil = file ?? settings.data.max_file_mb;
  const changes = [
    img !== settings.data.max_image_mb ? t("settings.fajllekeres.changeImage", { from: settings.data.max_image_mb, to: img }) : null,
    fil !== settings.data.max_file_mb ? t("settings.fajllekeres.changeFile", { from: settings.data.max_file_mb, to: fil }) : null,
  ].filter((c): c is string => c !== null);
  const ok = (n: number) => Number.isInteger(n) && n >= 1 && n <= 2048;
  return (
    <section className="flex flex-col gap-3">
      <p className="text-ui text-fg-muted">{t("settings.fajllekeres.body")}</p>
      <Field label={t("settings.fajllekeres.image")} hint={t("settings.mbRange")}>
        <Input type="number" min={1} max={2048} value={img} onChange={(e) => setImage(Number(e.target.value))} className="w-32" />
      </Field>
      <Field label={t("settings.fajllekeres.file")} hint={t("settings.mbRange")}>
        <Input type="number" min={1} max={2048} value={fil} onChange={(e) => setFile(Number(e.target.value))} className="w-32" />
      </Field>
      <Save
        changes={changes}
        disabled={!ok(img) || !ok(fil)}
        loading={save.isPending}
        onSave={() =>
          save.mutate(
            { max_image_mb: img, max_file_mb: fil },
            {
              onSuccess: () => {
                setImage(null);
                setFile(null);
                toast(t("settings.saved"));
              },
              onError: (e) => toast(e.message, { tone: "danger" }),
            },
          )
        }
      >
        {t("settings.save")}
      </Save>
    </section>
  );
}

function SliderField({ label, unit, min, max, value, onChange }: { label: string; unit: string; min: number; max: number; value: number; onChange: (n: number) => void }) {
  return (
    <div className="flex flex-col gap-1 text-ui">
      <span className="font-medium">{label}</span>
      <div className="flex items-center gap-3">
        <input type="range" aria-label={label} min={min} max={max} value={value} onChange={(e) => onChange(Number(e.target.value))} className="min-w-0 flex-1 accent-accent" />
        <Input type="number" aria-label={`${label} (${unit})`} min={min} max={max} value={value} onChange={(e) => onChange(Number(e.target.value))} className="w-24" />
        <span className="w-10 text-meta text-fg-subtle">{unit}</span>
      </div>
    </div>
  );
}

function DelegationSection() {
  const { t, i18n } = useTranslation("admin");
  const settings = useDelegationSettings();
  const save = useSetDelegationSettings();
  const grants = useActiveGrants();
  const revoke = useRevokeGrant();
  const [idle, setIdle] = useState<number | null>(null);
  const [hours, setHours] = useState<number | null>(null);
  if (settings.isPending) return <Skeleton className="h-32 w-full" />;
  if (settings.isError) return <p className="text-danger">{settings.error.message}</p>;
  const idleValue = idle ?? settings.data.grant_idle_minutes;
  const hoursValue = hours ?? settings.data.grant_max_hours;
  const changes = [
    idleValue !== settings.data.grant_idle_minutes ? t("settings.feladatatadas.changeIdle", { from: settings.data.grant_idle_minutes, to: idleValue }) : null,
    hoursValue !== settings.data.grant_max_hours ? t("settings.feladatatadas.changeMax", { from: settings.data.grant_max_hours, to: hoursValue }) : null,
  ].filter((c): c is string => c !== null);
  const valid = idleValue >= 5 && idleValue <= 1440 && hoursValue >= 1 && hoursValue <= 72;
  return (
    <section className="flex flex-col gap-4">
      <p className="text-ui text-fg-muted">{t("settings.feladatatadas.body")}</p>
      <SliderField label={t("settings.feladatatadas.idle")} unit={t("settings.minutes")} min={5} max={1440} value={idleValue} onChange={setIdle} />
      <SliderField label={t("settings.feladatatadas.max")} unit={t("settings.hours")} min={1} max={72} value={hoursValue} onChange={setHours} />
      <Save
        changes={changes}
        disabled={!valid}
        loading={save.isPending}
        onSave={() =>
          save.mutate(
            { grant_idle_minutes: idleValue, grant_max_hours: hoursValue },
            {
              onSuccess: () => {
                setIdle(null);
                setHours(null);
                toast(t("settings.saved"));
              },
              onError: (e) => toast(e.message, { tone: "danger" }),
            },
          )
        }
      >
        {t("settings.save")}
      </Save>
      <h2 className="mt-2 font-semibold">{t("settings.feladatatadas.grants")}</h2>
      {grants.data?.length ? (
        <ul className="flex flex-col divide-y divide-border">
          {grants.data.map((g) => (
            <li key={g.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-ui">
              <span className="min-w-0 flex-1 basis-56 truncate font-medium">
                @{g.agent_a_handle} ↔ @{g.agent_b_handle}
              </span>
              <span className="text-meta text-fg-muted">{t("settings.feladatatadas.expires", { when: fullTime(g.effective_expires_at, i18n.language), who: g.approver_handle })}</span>
              <Button size="sm" variant="ghost" loading={revoke.isPending} onClick={() => revoke.mutate(g.id, { onError: (e) => toast(e.message, { tone: "danger" }) })}>
                {t("settings.feladatatadas.revoke")}
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-ui text-fg-muted">{t("settings.feladatatadas.noGrants")}</p>
      )}
    </section>
  );
}

function YesNo({ on, label }: { on: boolean | undefined; label: string }) {
  const { t } = useTranslation("admin");
  const Icon = on ? CircleCheck : CircleMinus;
  return (
    <span className="inline-flex items-center gap-1.5 text-ui">
      <Icon aria-hidden className={cn("size-4", on ? "text-success" : "text-fg-subtle")} />
      {label}: <b>{t(on ? "settings.yes" : "settings.no")}</b>
    </span>
  );
}

function GiteaSection() {
  const { t, i18n } = useTranslation("admin");
  const status = useSystemStatus(true, false);
  const mappings = useGitMappings();
  const channels = useChannels();
  const add = useAddGitMapping();
  const remove = useRemoveGitMapping();
  const [repo, setRepo] = useState("");
  const [channelId, setChannelId] = useState("");
  const [confirm, setConfirm] = useState<string | null>(null);
  const list = (channels.data ?? []).filter((c) => c.kind !== "dm" && !c.archived_at);
  const name = (id: string) => channels.data?.find((c) => c.id === id)?.name ?? id.slice(0, 8);
  const repoBad = repo !== "" && !validRepo(repo);
  return (
    <section className="flex flex-col gap-3">
      <p className="flex flex-wrap gap-x-5 gap-y-1">
        <YesNo on={status.data?.system?.gitea_configured} label="COLONY_GITEA_URL" />
        <YesNo on={status.data?.system?.gitea_webhook_configured} label={t("settings.gitea.webhook")} />
      </p>
      <p className="text-ui text-fg-muted">{t("settings.gitea.body")}</p>
      {mappings.isPending ? (
        <Skeleton className="h-16 w-full" />
      ) : mappings.isError ? (
        <p className="text-danger">{mappings.error.message}</p>
      ) : mappings.data.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-ui">
            <thead className="border-b border-border text-left text-meta text-fg-subtle">
              <tr>
                <th className="px-2 py-1.5 font-medium">{t("settings.gitea.repo")}</th>
                <th className="px-2 py-1.5 font-medium">{t("settings.gitea.channel")}</th>
                <th className="px-2 py-1.5 font-medium">{t("settings.gitea.since")}</th>
                <th className="w-10" />
              </tr>
            </thead>
            <tbody>
              {mappings.data.map((m) => (
                <tr key={m.repo} className="border-b border-border/60 last:border-0">
                  <td className="px-2 py-1 font-mono text-meta break-all">{m.repo}</td>
                  <td className="px-2">#{name(m.channel_id)}</td>
                  <td className="px-2 whitespace-nowrap text-fg-muted">{fullTime(m.created_at, i18n.language)}</td>
                  <td className="px-2 text-right">
                    <Button size="sm" variant="ghost" aria-label={t("settings.gitea.removeOne", { repo: m.repo })} icon={<Trash2 className="size-4" />} onClick={() => setConfirm(m.repo)} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <EmptyState title={t("settings.gitea.empty")} />
      )}
      <form
        className="flex flex-wrap items-end gap-2 border-t border-border pt-3"
        onSubmit={(e) => {
          e.preventDefault();
          add.mutate(
            { repo, channel_id: channelId },
            {
              onSuccess: () => {
                setRepo("");
                toast(t("settings.saved"));
              },
              onError: (err) => toast(err.message, { tone: "danger" }),
            },
          );
        }}
      >
        <Field label={t("settings.gitea.repo")} hint={repoBad ? t("settings.gitea.repoBad") : undefined}>
          <Input value={repo} placeholder="owner/repo" aria-invalid={repoBad} onChange={(e) => setRepo(e.target.value.trim())} className="w-56 max-w-full font-mono" />
        </Field>
        <Field label={t("settings.gitea.channel")}>
          <Select value={channelId} onChange={(e) => setChannelId(e.target.value)} className="w-48 max-w-full">
            <option value="">{t("settings.gitea.pick")}</option>
            {list.map((c) => (
              <option key={c.id} value={c.id}>
                #{c.name}
              </option>
            ))}
          </Select>
        </Field>
        <Button type="submit" variant="primary" disabled={!validRepo(repo) || !channelId} loading={add.isPending}>
          {t("settings.gitea.add")}
        </Button>
      </form>
      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t("settings.gitea.removeTitle", { repo: confirm ?? "" })}
        closeLabel={t("close")}
        footer={
          <Button
            variant="danger"
            loading={remove.isPending}
            onClick={() => confirm && remove.mutate(confirm, { onSuccess: () => setConfirm(null), onError: (e) => toast(e.message, { tone: "danger" }) })}
          >
            {t("settings.gitea.remove")}
          </Button>
        }
      >
        <p>{t("settings.gitea.removeBody")}</p>
      </Dialog>
    </section>
  );
}

const emptyPrice = { model: "", input_per_mtok: "", output_per_mtok: "", cache_read_per_mtok: "", cache_write_per_mtok: "" };

function PricesSection() {
  const { t, i18n } = useTranslation("admin");
  const locale = localeOf(i18n.language);
  const prices = usePrices();
  const add = useAddPrice();
  const currency = useCurrency();
  const setCurrency = useSetCurrency();
  const [draft, setDraft] = useState(emptyPrice);
  const [rateDraft, setRate] = useState<string | null>(null);
  const rate = rateDraft ?? (currency.data?.usd_huf ? String(currency.data.usd_huf) : "");
  const values = priceFields.map((f) => Number(draft[f]));
  const priceOk = draft.model.trim() !== "" && priceFields.every((f) => draft[f] !== "") && values.every((v) => Number.isFinite(v) && v >= 0);
  const rateValue = Number(rate);
  const rateChanges = rate !== "" && rateValue > 0 && rateValue !== currency.data?.usd_huf ? [t("settings.arak.changeRate", { from: currency.data?.usd_huf ?? "–", to: rateValue })] : [];
  const money = (n: number) => `$${n.toLocaleString(locale, { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`;
  const save = () =>
    add.mutate(
      {
        model: draft.model.trim(),
        input_per_mtok: Number(draft.input_per_mtok),
        output_per_mtok: Number(draft.output_per_mtok),
        cache_read_per_mtok: Number(draft.cache_read_per_mtok),
        cache_write_per_mtok: Number(draft.cache_write_per_mtok),
      },
      {
        onSuccess: () => {
          setDraft(emptyPrice);
          toast(t("settings.saved"));
        },
        onError: (e) => toast(e.message, { tone: "danger" }),
      },
    );
  return (
    <section className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <h2 className="font-semibold">{t("settings.arak.rateTitle")}</h2>
        <Field label={t("settings.arak.rate")} hint={currency.data?.updated_at ? t("settings.arak.rateChanged", { when: fullTime(currency.data.updated_at, i18n.language) }) : t("settings.arak.rateNever")}>
          <Input type="number" min={0} step="0.01" value={rate} onChange={(e) => setRate(e.target.value)} className="w-36" />
        </Field>
        <Save
          changes={rateChanges}
          loading={setCurrency.isPending}
          onSave={() => setCurrency.mutate(rateValue, { onSuccess: () => { setRate(null); toast(t("settings.saved")); }, onError: (e) => toast(e.message, { tone: "danger" }) })}
        >
          {t("settings.save")}
        </Save>
      </div>
      <div className="flex flex-col gap-3 border-t border-border pt-4">
        <h2 className="font-semibold">{t("settings.arak.title")}</h2>
        <Notice>{t("settings.arak.note")}</Notice>
        {prices.isPending ? (
          <Skeleton className="h-16 w-full" />
        ) : prices.isError ? (
          <p className="text-danger">{prices.error.message}</p>
        ) : prices.data.length ? (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[40rem] text-ui">
              <thead className="border-b border-border text-right text-meta text-fg-subtle">
                <tr>
                  <th className="px-2 py-1.5 text-left font-medium">{t("settings.arak.model")}</th>
                  {priceFields.map((f) => (
                    <th key={f} className="px-2 py-1.5 font-medium whitespace-nowrap">
                      {t(`settings.arak.${f}`)}
                    </th>
                  ))}
                  <th className="px-2 py-1.5 font-medium whitespace-nowrap">{t("settings.arak.validFrom")}</th>
                </tr>
              </thead>
              <tbody className="tabular">
                {priceHistory(prices.data).flatMap((g) =>
                  g.rows.map((p, i) => (
                    <tr key={p.id} className={cn("border-b border-border/60 last:border-0", i > 0 && "text-fg-subtle")}>
                      <td className="px-2 py-1 font-mono text-meta whitespace-nowrap">
                        {p.model}
                        {i > 0 ? <span className="ml-2 font-sans">{t("settings.arak.history")}</span> : null}
                      </td>
                      {priceFields.map((f: PriceField) => (
                        <td key={f} className="px-2 text-right">
                          {money(p[f])}
                        </td>
                      ))}
                      <td className="px-2 text-right whitespace-nowrap">{fullTime(p.valid_from, i18n.language)}</td>
                    </tr>
                  )),
                )}
              </tbody>
            </table>
          </div>
        ) : (
          <EmptyState title={t("settings.arak.empty")} body={t("settings.arak.emptyBody")} />
        )}
        <form
          className="flex flex-col gap-3 border-t border-border pt-3"
          onSubmit={(e) => {
            e.preventDefault();
            save();
          }}
        >
          <h3 className="font-semibold">{t("settings.arak.new")}</h3>
          <Field label={t("settings.arak.model")} hint={t("settings.arak.modelHint")}>
            <Input value={draft.model} onChange={(e) => setDraft({ ...draft, model: e.target.value })} className="w-72 max-w-full font-mono" />
          </Field>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {priceFields.map((f) => (
              <Field key={f} label={t(`settings.arak.${f}`)}>
                <Input type="number" min={0} step="0.01" value={draft[f]} onChange={(e) => setDraft({ ...draft, [f]: e.target.value })} />
              </Field>
            ))}
          </div>
          <div>
            <Button type="submit" variant="primary" disabled={!priceOk} loading={add.isPending}>
              {t("settings.arak.add")}
            </Button>
          </div>
        </form>
      </div>
    </section>
  );
}

function FilesSection() {
  const { t } = useTranslation("admin");
  const status = useSystemStatus(true, false);
  return (
    <section className="flex flex-col gap-2 text-ui">
      <p className="text-fg-muted">{t("settings.fajlok.body")}</p>
      <p>
        <span className="font-mono text-meta">COLONY_MAX_UPLOAD_MB</span>: <b className="tabular">{status.data?.system?.max_upload_mb ?? "–"} MB</b>
      </p>
      <div className="max-w-md">
        <StorageCard />
      </div>
    </section>
  );
}
