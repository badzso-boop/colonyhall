// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { Fragment, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { GitCompare, History, Pencil, ScrollText } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { useWidth } from "@/shared/hooks";
import { cn, relativeTime } from "@/shared/lib";
import { Button, Dialog, DiffView, EmptyState, Markdown, PageHeader, Select, Sheet, Skeleton, Tab, TabList, TabPanel, Tabs, Textarea, Tooltip, toast } from "@/shared/ui";
import { useRules, useRulesVersion, useSaveRules } from "../api";
import { MaxLen, lengthState, placeholders, renderRules, segments, type Placeholder } from "../model";

function Chip({ name }: { name: Placeholder }) {
  const { t } = useTranslation("rules");
  return (
    <Tooltip content={t(`ph.${name}`)}>
      <span tabIndex={0} className="mx-0.5 inline-flex rounded-sm bg-info-soft px-1 font-mono text-meta text-info">
        {`{${name}}`}
      </span>
    </Tooltip>
  );
}

/** The rules as read: markdown per block, placeholders as chips inside the plain-text paragraphs. */
function Reading({ text }: { text: string }) {
  const { t } = useTranslation("rules");
  // Markdown cannot hold React chips; split the text into paragraphs and render
  // the ones with placeholders as text with chips, the rest as markdown.
  const blocks = text.split(/\n{2,}/);
  return (
    <div className="flex flex-col gap-3">
      {blocks.map((b, i) =>
        /\{(handle|admins|workspace)\}/.test(b) ? (
          <p key={i} className="whitespace-pre-wrap">
            {segments(b).map((s, j) => ("placeholder" in s ? <Chip key={j} name={s.placeholder} /> : <Fragment key={j}>{s.text}</Fragment>))}
          </p>
        ) : (
          <Markdown key={i} text={b} copyLabel={t("copy")} copiedLabel={t("copied")} />
        ),
      )}
    </div>
  );
}

/** Szabályok (round 32, part 9): read, edit as a new version, compare versions, preview as an agent. */
export default function RulesPage() {
  const { t, i18n } = useTranslation("rules");
  const locale = localeOf(i18n.language);
  const narrow = useWidth() === "narrow";
  const rules = useRules();
  const actors = useActors();
  const session = useSession();
  const save = useSaveRules();
  const role = session.data?.actor.role;
  const admin = role === "owner" || role === "admin";

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [confirm, setConfirm] = useState<"save" | "leave" | null>(null);
  const [as, setAs] = useState("");
  const [compare, setCompare] = useState<number | null>(null);
  const [timelineOpen, setTimelineOpen] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const other = useRulesVersion(compare);

  const current = rules.data;
  const dirty = editing && current !== undefined && draft !== current.text;
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const agents = useMemo(() => [...(actors.data?.values() ?? [])].filter((a) => a.kind === "agent").sort((a, b) => a.handle.localeCompare(b.handle)), [actors.data]);
  const admins = useMemo(
    () => [...(actors.data?.values() ?? [])].filter((a) => a.kind === "human" && (a.role === "owner" || a.role === "admin")).map((a) => a.handle).sort(),
    [actors.data],
  );
  const shown = editing ? draft : (current?.text ?? "");
  const previewText = as ? renderRules(shown, actors.data?.get(as)?.handle ?? "", admins, "Colonyhall") : null;

  const insert = (p: Placeholder) => {
    const el = area.current;
    const at = el ? el.selectionStart : draft.length;
    const end = el ? el.selectionEnd : draft.length;
    const next = `${draft.slice(0, at)}{${p}}${draft.slice(end)}`;
    setDraft(next);
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(at + p.length + 2, at + p.length + 2);
    });
  };
  const doSave = () =>
    save.mutate(draft, {
      onSuccess: (r) => {
        toast(t("saved", { version: r.version }));
        setConfirm(null);
        setEditing(false);
      },
      onError: (e) => toast(e.message, { tone: "danger" }),
    });

  const len = [...draft].length;
  const lstate = lengthState(draft);
  const editor = (
    <div className="flex flex-col gap-2">
      <div role="toolbar" aria-label={t("insert")} className="flex flex-wrap items-center gap-1">
        <span className="text-meta text-fg-subtle">{t("insert")}:</span>
        {placeholders.map((p) => (
          <button key={p} type="button" title={t(`ph.${p}`)} onClick={() => insert(p)} className="rounded-sm bg-info-soft px-1.5 py-0.5 font-mono text-meta text-info hover:underline">
            {`{${p}}`}
          </button>
        ))}
        <span className={cn("ml-auto text-meta tabular-nums", lstate === "near" && "font-semibold text-warning", lstate === "over" && "font-semibold text-danger")}>
          {t("count", { n: len, max: MaxLen })}
        </span>
      </div>
      <Textarea ref={area} aria-label={t("editor")} value={draft} onChange={(e) => setDraft(e.target.value)} rows={24} className="font-mono text-ui" />
      {lstate === "over" ? <p className="text-meta text-danger">{t("tooLong", { max: MaxLen })}</p> : null}
    </div>
  );
  const preview = (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2">
        <Select aria-label={t("previewAs")} value={as} onChange={(e) => setAs(e.target.value)} className="w-56">
          <option value="">{t("previewNone")}</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              @{a.handle}
            </option>
          ))}
        </Select>
        {as ? <span className="text-meta text-fg-subtle">{t("previewHint")}</span> : null}
      </div>
      {previewText !== null ? <Markdown text={previewText} copyLabel={t("copy")} copiedLabel={t("copied")} /> : <Reading text={shown} />}
    </div>
  );

  const timeline = current ? (
    <ol className="flex flex-col gap-1">
      {[...current.history].sort((a, b) => b.version - a.version).map((h) => {
        const who = h.created_by ? actors.data?.get(h.created_by) : undefined;
        const isCurrent = h.version === current.version;
        return (
          <li key={h.version} className={cn("flex items-center gap-2 rounded-md px-2 py-1.5 text-ui", compare === h.version && "bg-accent-soft")}>
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="font-semibold">
                {t("version", { version: h.version })} {isCurrent ? <span className="ml-1 text-meta font-normal text-accent">· {t("current")}</span> : null}
              </span>
              <span className="text-meta text-fg-subtle">
                {who ? t("by", { handle: who.handle }) : ""} {h.created_at ? `· ${relativeTime(h.created_at, new Date(), locale)}` : ""}
              </span>
            </span>
            {!isCurrent ? (
              <Button size="sm" variant="ghost" icon={<GitCompare className="size-3.5" />} aria-label={t("compareWith", { version: h.version })} onClick={() => setCompare(h.version)}>
                {t("compare")}
              </Button>
            ) : null}
          </li>
        );
      })}
    </ol>
  ) : null;

  return (
    <div className="flex h-full flex-col">
      <PageHeader
        icon={<ScrollText className="size-5" />}
        title={t("title")}
        actions={
          <>
            {narrow && current ? (
              <Button size="sm" icon={<History className="size-4" />} onClick={() => setTimelineOpen(true)}>
                {t("timeline")}
              </Button>
            ) : null}
            {editing ? (
              <>
                <Button size="sm" variant="ghost" onClick={() => (dirty ? setConfirm("leave") : setEditing(false))}>
                  {t("cancel")}
                </Button>
                <Button size="sm" variant="primary" disabled={!dirty || lstate === "over"} onClick={() => setConfirm("save")}>
                  {t("save")}
                </Button>
              </>
            ) : admin && current ? (
              <Button
                size="sm"
                icon={<Pencil className="size-4" />}
                onClick={() => {
                  setDraft(current.text);
                  setEditing(true);
                }}
              >
                {t("edit")}
              </Button>
            ) : null}
          </>
        }
      />
      <div className="flex min-h-0 flex-1">
        <div className="min-h-0 flex-1 overflow-y-auto p-4">
          {rules.isPending ? <Skeleton className="h-40 w-full" /> : null}
          {current && !current.text && !editing ? <EmptyState icon={<ScrollText className="size-8" />} title={t("empty")} /> : null}
          {current && editing ? (
            narrow ? (
              <Tabs defaultValue="edit">
                <TabList label={t("editor")}>
                  <Tab value="edit">{t("editor")}</Tab>
                  <Tab value="preview">{t("preview")}</Tab>
                </TabList>
                <TabPanel value="edit" className="pt-2">
                  {editor}
                </TabPanel>
                <TabPanel value="preview" className="pt-2">
                  {preview}
                </TabPanel>
              </Tabs>
            ) : (
              <div className="grid grid-cols-2 gap-4">
                {editor}
                <section aria-label={t("preview")} className="rounded-lg border border-border p-3">
                  {preview}
                </section>
              </div>
            )
          ) : current?.text ? (
            <div className="mx-auto flex max-w-3xl flex-col gap-3">
              <p className="text-meta text-fg-subtle">
                {t("version", { version: current.version })} · {t("current")}
                {!admin ? ` · ${t("readOnly")}` : ""}
              </p>
              {preview}
            </div>
          ) : null}
        </div>
        {!narrow && current ? (
          <aside aria-label={t("timeline")} className="w-72 shrink-0 overflow-y-auto border-l border-border p-3">
            <h2 className="mb-2 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t("timeline")}</h2>
            {timeline}
          </aside>
        ) : null}
      </div>
      {narrow ? (
        <Sheet open={timelineOpen} onOpenChange={setTimelineOpen} title={t("timeline")} closeLabel={t("close")}>
          {timeline}
        </Sheet>
      ) : null}
      <Dialog
        open={compare !== null}
        onOpenChange={(o) => !o && setCompare(null)}
        title={t("diffTitle", { a: compare ?? 0, b: current?.version ?? 0 })}
        size="xl"
        closeLabel={t("close")}
      >
        {other.data && current ? <DiffView a={other.data.text} b={current.text} skipLabel={(n) => t("skip", { count: n })} className="max-h-[60dvh]" /> : <Skeleton className="h-40 w-full" />}
      </Dialog>
      <Dialog
        open={confirm === "save"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t("confirmTitle")}
        description={t("confirmBody")}
        size="xl"
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>{t("cancel")}</Button>
            <Button variant="primary" loading={save.isPending} onClick={doSave}>
              {t("save")}
            </Button>
          </>
        }
      >
        {current ? <DiffView a={current.text} b={draft} skipLabel={(n) => t("skip", { count: n })} className="max-h-[60dvh]" /> : null}
      </Dialog>
      <Dialog
        open={confirm === "leave"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t("leave")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>{t("keep")}</Button>
            <Button
              variant="danger"
              onClick={() => {
                setConfirm(null);
                setEditing(false);
              }}
            >
              {t("discard")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </div>
  );
}
