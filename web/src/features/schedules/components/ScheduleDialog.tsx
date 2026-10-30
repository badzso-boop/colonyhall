import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { conversationTitle, useChannels } from "@/features/conversation";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { cn, type ScheduleDraft } from "@/shared/lib";
import { Button, Dialog, IconButton, Input, Select, Switch, Textarea, toast } from "@/shared/ui";
import { useCreateSchedule, useSchedulePreview } from "../api";
import { buildCron, describeRule, parseCron, quickChips, weekOrder, type Mode, type Rule } from "../rule";

const DefaultZone = "Europe/Budapest";
const modes: Mode[] = ["once", "daily", "weekly", "monthly", "advanced"];

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setV(value), ms);
    return () => window.clearTimeout(id);
  }, [value, ms]);
  return v;
}

function zones(): string[] {
  try {
    return Intl.supportedValuesOf("timeZone");
  } catch {
    return [DefaultZone, "UTC"];
  }
}

function validZone(z: string): boolean {
  try {
    new Intl.DateTimeFormat("en", { timeZone: z });
    return true;
  } catch {
    return false;
  }
}

/** The starting rule: the draft's cron read back, or a one-off tomorrow at 09:00. */
function initialRule(draft: ScheduleDraft): Rule {
  if (draft.cron) return parseCron(draft.cron);
  const tomorrow = quickChips(new Date()).find((c) => c.id === "tomorrow")!;
  return { mode: "once", date: tomorrow.date, time: tomorrow.time };
}

function blank(mode: Mode, prev: Rule): Rule {
  const time = "time" in prev ? prev.time : "09:00";
  switch (mode) {
    case "once":
      return { mode, date: quickChips(new Date()).find((c) => c.id === "tomorrow")!.date, time };
    case "daily":
      return { mode, time, workdays: false };
    case "weekly":
      return { mode, days: [1], time };
    case "monthly":
      return { mode, day: 1, time };
    case "advanced": {
      const built = buildCron(prev);
      return { mode, cron: built && "cron" in built ? built.cron : "0 9 * * *" };
    }
  }
}

function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="flex flex-col gap-1">
      <span className="text-meta font-semibold text-fg-muted">{label}</span>
      {children}
      {hint ? <span className="text-meta text-fg-subtle">{hint}</span> : null}
    </label>
  );
}

/**
 * The schedule builder (docs/handoff/32, part 6): what, where, when (no cron
 * typing unless Haladó), time zone and catch-up; the summary and the next
 * runs come from the server's preview, so the two never disagree.
 */
export function ScheduleDialog({ draft, onClose }: { draft: ScheduleDraft; onClose: () => void }) {
  const { t, i18n } = useTranslation("schedules");
  const locale = localeOf(i18n.language);
  const channels = useChannels();
  const actors = useActors();
  const session = useSession();
  const create = useCreateSchedule();
  const [body, setBody] = useState(draft.body);
  const [chosen, setChannelId] = useState(draft.channelId ?? "");
  const [thread, setThread] = useState(draft.threadRootId);
  const [rule, setRule] = useState<Rule>(() => initialRule(draft));
  const [zone, setZone] = useState(draft.timezone ?? DefaultZone);
  const [catchUp, setCatchUp] = useState<"once" | "skip">(draft.catchUp ?? "once");
  const allZones = useMemo(() => zones(), []);

  const meId = session.data?.actor.id ?? "";
  // The server posts in the creator's name, so only conversations they are in.
  const open = useMemo(() => (channels.data ?? []).filter((c) => c.is_member || c.id === draft.channelId), [channels.data, draft.channelId]);
  const channelId = chosen || open[0]?.id || "";
  const channel = open.find((c) => c.id === channelId);
  const channelName = channel && actors.data ? conversationTitle(channel, actors.data, meId) : "";

  const built = buildCron(rule);
  const zoneOk = validZone(zone);
  const previewBody = useDebounced(built && zoneOk ? { ...built, timezone: zone } : null, 300);
  const preview = useSchedulePreview(previewBody);
  const ready = Boolean(built && zoneOk && body.trim() && channelId && !preview.isError);

  const save = () => {
    if (!built || !ready) return;
    create.mutate(
      { channel_id: channelId, ...(thread ? { thread_root_id: thread } : {}), body: body.trim(), ...built, timezone: zone, catch_up: catchUp },
      {
        onSuccess: (s) => {
          toast(t("saved", { when: new Date(s.next_run_at).toLocaleString(locale) }));
          draft.onSaved?.();
          onClose();
        },
        onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }),
      },
    );
  };

  const set = (patch: Partial<Rule>) => setRule((r) => ({ ...r, ...patch }) as Rule);
  const timeInput = "time" in rule && (
    <Field label={t("time")}>
      <Input type="time" step={900} value={rule.time} onChange={(e) => set({ time: e.target.value })} className="w-32" />
    </Field>
  );
  const runs = preview.data?.next_runs ?? [];
  const fmtRun = (iso: string) =>
    new Date(iso).toLocaleString(locale, { timeZone: zoneOk ? zone : undefined, weekday: "short", year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("builder")}
      size="lg"
      closeLabel={t("close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("cancel")}
          </Button>
          <Button variant="primary" disabled={!ready} loading={create.isPending} onClick={save}>
            {t("save")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-1 font-semibold">{t("what")}</legend>
          <Field label={t("message")}>
            <Textarea rows={3} value={body} onChange={(e) => setBody(e.target.value)} />
          </Field>
          <Field label={t("channel")}>
            <Select
              value={channelId}
              onChange={(e) => {
                setChannelId(e.target.value);
                setThread(undefined);
              }}
            >
              {open.map((c) => (
                <option key={c.id} value={c.id}>
                  {actors.data ? conversationTitle(c, actors.data, meId) : c.name}
                </option>
              ))}
            </Select>
          </Field>
          {thread ? (
            <p className="flex items-center gap-1 self-start rounded-full bg-surface-sunken py-0.5 pr-0.5 pl-2.5 text-meta">
              {t("inThread")}
              <IconButton size="sm" label={t("removeThread")} icon={<X className="size-3" />} onClick={() => setThread(undefined)} />
            </p>
          ) : null}
        </fieldset>

        <fieldset className="flex flex-col gap-3">
          <legend className="mb-1 font-semibold">{t("when")}</legend>
          <div role="radiogroup" aria-label={t("when")} className="flex flex-wrap rounded-md border border-border p-0.5">
            {modes.map((m) => (
              <button
                key={m}
                type="button"
                role="radio"
                aria-checked={rule.mode === m}
                onClick={() => rule.mode !== m && setRule(blank(m, rule))}
                className={cn("flex-1 rounded-sm px-2.5 py-1 text-ui", rule.mode === m ? "bg-accent-soft font-semibold text-fg" : "text-fg-muted")}
              >
                {t(`modes.${m}`)}
              </button>
            ))}
          </div>
          {rule.mode === "once" ? (
            <>
              <div className="flex flex-wrap gap-1.5">
                {quickChips(new Date()).map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => setRule({ mode: "once", date: c.date, time: c.time })}
                    className="rounded-full border border-border px-2.5 py-0.5 text-meta hover:bg-surface-sunken"
                  >
                    {t(`chips.${c.id}`)}
                  </button>
                ))}
              </div>
              <div className="flex flex-wrap gap-3">
                <Field label={t("date")}>
                  <Input type="date" value={rule.date} onChange={(e) => set({ date: e.target.value })} className="w-44" />
                </Field>
                {timeInput}
              </div>
            </>
          ) : null}
          {rule.mode === "daily" ? (
            <div className="flex flex-wrap items-end gap-4">
              {timeInput}
              <label className="flex h-8 items-center gap-2 text-ui">
                <Switch checked={rule.workdays} onCheckedChange={(v) => set({ workdays: v })} />
                {t("workdays")}
              </label>
            </div>
          ) : null}
          {rule.mode === "weekly" ? (
            <>
              <div role="group" aria-label={t("days")} className="flex flex-wrap gap-1">
                {weekOrder.map((d) => {
                  const on = rule.days.includes(d);
                  return (
                    <button
                      key={d}
                      type="button"
                      aria-pressed={on}
                      aria-label={new Date(2026, 9, 25 + d).toLocaleDateString(locale, { weekday: "long" })}
                      onClick={() => set({ days: on ? rule.days.filter((x) => x !== d) : [...rule.days, d] })}
                      className={cn("h-8 min-w-10 rounded-md border px-2 text-ui", on ? "border-accent bg-accent-soft font-semibold" : "border-border text-fg-muted")}
                    >
                      {new Date(2026, 9, 25 + d).toLocaleDateString(locale, { weekday: "short" })}
                    </button>
                  );
                })}
              </div>
              {timeInput}
            </>
          ) : null}
          {rule.mode === "monthly" ? (
            <div className="flex flex-wrap gap-3">
              <Field label={t("dayOfMonth")} hint={rule.day > 28 ? t("monthlyHint") : undefined}>
                <Input type="number" min={1} max={31} value={rule.day} onChange={(e) => set({ day: Number(e.target.value) })} className="w-24" />
              </Field>
              {timeInput}
            </div>
          ) : null}
          {rule.mode === "advanced" ? (
            <Field label={t("cron")} hint={t("cronHint")}>
              <Input value={rule.cron} onChange={(e) => set({ cron: e.target.value })} className="font-mono" spellCheck={false} />
            </Field>
          ) : null}
        </fieldset>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label={t("timezone")}>
            <Input list="colony-zones" value={zone} onChange={(e) => setZone(e.target.value)} aria-invalid={!zoneOk} />
            <datalist id="colony-zones">
              {allZones.map((z) => (
                <option key={z} value={z} />
              ))}
            </datalist>
          </Field>
          <div role="radiogroup" aria-label={t("catchUp")} className="flex flex-col gap-1">
            <span className="text-meta font-semibold text-fg-muted">{t("catchUp")}</span>
            <div className="flex gap-3 text-ui">
              {(["once", "skip"] as const).map((c) => (
                <label key={c} className="flex items-center gap-1.5">
                  <input type="radio" name="catch_up" checked={catchUp === c} onChange={() => setCatchUp(c)} className="accent-(--color-accent)" />
                  {t(c === "once" ? "catchOnce" : "catchSkip")}
                </label>
              ))}
            </div>
            <span className="text-meta text-fg-subtle">{t("catchHint")}</span>
          </div>
        </div>

        <section aria-live="polite" className="rounded-md border border-border bg-surface-sunken p-3">
          <p className="font-semibold">
            {describeRule(rule, t, locale)} ({zone}) {channelName ? t(thread ? "toThread" : "to", { name: channelName }) : ""}
          </p>
          {preview.isError ? (
            <p className="mt-1 text-ui text-danger">{t("previewError", { error: preview.error.message })}</p>
          ) : runs.length ? (
            <>
              <p className="mt-2 text-meta font-semibold text-fg-muted">{runs.length === 1 ? t("nextOne") : t("next")}</p>
              <ol className="text-ui tabular-nums">
                {runs.map((r) => (
                  <li key={r}>{fmtRun(r)}</li>
                ))}
              </ol>
            </>
          ) : null}
          {preview.data?.cron ? <p className="mt-2 font-mono text-meta text-fg-subtle">{preview.data.cron}</p> : null}
        </section>
      </div>
    </Dialog>
  );
}
