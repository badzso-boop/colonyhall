// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { RoutinesPanel } from "./RoutinesPanel";
import { useTranslation } from "react-i18next";
import { Download, Plus, Puzzle, Trash2, Upload } from "lucide-react";
import { fullTime } from "@/shared/lib";
import {
  Badge,
  Button,
  Dialog,
  EmptyState,
  IconButton,
  Input,
  Notice,
  Select,
  Skeleton,
  Switch,
  Textarea,
  toast,
} from "@/shared/ui";
import {
  useAddVersion,
  useApproveVersion,
  useCreateExtension,
  exportExtension,
  useDeleteExtension,
  useImportExtension,
  useExtensions,
  useExtensionVersion,
  useRollback,
  useSetExtensionApprovalOff,
  useSetExtensionEnabled,
  useTakeOver,
  useTestTool,
  type Bundle,
  type Extension,
  type ExtensionDiff,
} from "../api";
import {
  diffIsEmpty,
  manifestTemplate,
  parseObject,
  slugFrom,
  sourceTemplate,
  validSlug,
} from "../model";

const stateTone = {
  draft: "neutral",
  live: "success",
  disabled: "warning",
} as const;
const riskTone = {
  read: "neutral",
  write: "warning",
  external: "danger",
} as const;

/** The workspace's extensions: create, version, test, approve, roll back, switch off. */
export function ExtensionsTab({ owner }: { owner: boolean }) {
  const { t } = useTranslation("workshop");
  const list = useExtensions();
  const [creating, setCreating] = useState(false);
  if (list.isPending) return <Skeleton className="h-24 w-full" />;
  if (list.isError)
    return <p className="text-ui text-danger">{list.error.message}</p>;
  const { extensions, runner_available: runner } = list.data;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <span className="mr-auto" />
        <Button
          size="sm"
          variant="primary"
          icon={<Plus className="size-4" />}
          onClick={() => setCreating(true)}
        >
          {t("extensions.new")}
        </Button>
        <ImportButton />
      </div>
      {!runner ? <Notice tone="info">{t("extensions.noRunner")}</Notice> : null}
      {extensions.length === 0 ? (
        <EmptyState
          icon={<Puzzle className="size-5" />}
          title={t("extensions.empty")}
          body={t("extensions.emptyBody")}
        />
      ) : (
        <ul className="flex flex-col gap-3">
          {extensions.map((x) => (
            <ExtensionCard key={x.id} x={x} owner={owner} />
          ))}
        </ul>
      )}
      {creating ? <CreateDialog onClose={() => setCreating(false)} /> : null}
    </div>
  );
}

function ExtensionCard({ x, owner }: { x: Extension; owner: boolean }) {
  const { t, i18n } = useTranslation("workshop");
  const enabled = useSetExtensionEnabled(x.id);
  const approval = useSetExtensionApprovalOff(x.id);
  const takeOver = useTakeOver(x.id);
  const [editing, setEditing] = useState(false);
  const [opened, setOpened] = useState<number | null>(null);
  const [deleting, setDeleting] = useState(false);
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  return (
    <li className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-3">
      <div className="flex flex-wrap items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold">{x.title}</span>
            <code className="font-mono text-meta text-fg-subtle">{x.slug}</code>
            <Badge tone={stateTone[x.state]}>
              {t(`extensions.state.${x.state}`)}
            </Badge>
            {x.live_version ? (
              <Badge tone="neutral">
                {t("extensions.live", { version: x.live_version })}
              </Badge>
            ) : null}
            {x.approval_off ? (
              <Badge tone="warning">{t("connectors.approvalOff")}</Badge>
            ) : null}
          </div>
          {x.description ? (
            <p className="mt-1 text-ui text-fg-muted">{x.description}</p>
          ) : null}
          <p className="text-meta text-fg-muted">
            {x.bot_handle
              ? t("extensions.bot", { handle: x.bot_handle })
              : null}
            {x.maintainer_handle ? (
              <span className="ml-3">
                {t("extensions.maintainer", { handle: x.maintainer_handle })}
              </span>
            ) : null}
          </p>
          {x.disabled_reason ? (
            <Notice tone="warning">
              {t("extensions.disabledReason", { reason: x.disabled_reason })}
            </Notice>
          ) : null}
        </div>
        <div className="flex gap-1">
          <Button
            size="sm"
            variant="secondary"
            icon={<Plus className="size-4" />}
            onClick={() => setEditing(true)}
          >
            {t("extensions.newVersion")}
          </Button>
          <IconButton
            label={t("extensions.export")}
            icon={<Download className="size-4" />}
            onClick={() =>
              exportExtension(x.id)
                .then((b) => {
                  const url = URL.createObjectURL(
                    new Blob([JSON.stringify(b, null, 2)], {
                      type: "application/json",
                    }),
                  );
                  const a = document.createElement("a");
                  a.href = url;
                  a.download = `${b.slug}-v${b.version}.json`;
                  a.click();
                  URL.revokeObjectURL(url);
                })
                .catch((e: Error) => toast(e.message, { tone: "danger" }))
            }
          />
          <IconButton
            label={t("extensions.delete")}
            icon={<Trash2 className="size-4" />}
            onClick={() => setDeleting(true)}
          />
        </div>
      </div>

      <div className="flex flex-wrap gap-x-6 gap-y-2">
        <label className="flex items-center gap-2 text-ui">
          <Switch
            checked={x.state === "live"}
            disabled={!x.live_version || enabled.isPending}
            onCheckedChange={(on) => enabled.mutate(on, { onError: fail })}
            aria-label={t("extensions.enabled")}
          />
          {t("extensions.enabled")}
        </label>
        <label
          className="flex items-center gap-2 text-ui"
          title={t("extensions.approvalHint")}
        >
          <Switch
            checked={x.approval_off}
            disabled={!owner || approval.isPending}
            onCheckedChange={(off) => approval.mutate(off, { onError: fail })}
            aria-label={t("extensions.approvalLabel")}
          />
          {t("extensions.approvalLabel")}
        </label>
        <Button
          size="sm"
          variant="ghost"
          title={t("extensions.takeOverHint")}
          loading={takeOver.isPending}
          onClick={() => takeOver.mutate(undefined, { onError: fail })}
        >
          {t("extensions.takeOver")}
        </Button>
      </div>

      {x.versions.length ? (
        <details open={x.state === "draft"}>
          <summary className="cursor-pointer text-ui font-medium">
            {t("extensions.versions")}
          </summary>
          <ul className="mt-2 flex flex-col divide-y divide-border rounded-md border border-border">
            {x.versions.map((v) => (
              <li
                key={v.version}
                className="flex flex-wrap items-center gap-2 px-2 py-1.5 text-ui"
              >
                <span className="font-mono">v{v.version}</span>
                {v.version === x.live_version ? (
                  <Badge tone="success">{t("extensions.state.live")}</Badge>
                ) : null}
                {v.approved ? (
                  <span className="text-meta text-fg-subtle">
                    {t("extensions.approved")}
                  </span>
                ) : null}
                <span className="text-meta text-fg-muted">
                  {fullTime(v.created_at, i18n.language)}
                </span>
                <span className="flex min-w-0 flex-1 flex-wrap gap-1">
                  {v.tools.map((tool) => (
                    <Badge key={tool.name} tone={riskTone[tool.risk]}>
                      {tool.name}
                    </Badge>
                  ))}
                </span>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setOpened(v.version)}
                >
                  {t("extensions.open")}
                </Button>
              </li>
            ))}
          </ul>
        </details>
      ) : null}

      {x.live_version ? <RoutinesPanel id={x.id} /> : null}
      {editing ? (
        <VersionEditor
          x={x}
          onClose={() => setEditing(false)}
          onSaved={(v) => setOpened(v)}
        />
      ) : null}
      {opened ? (
        <VersionDialog x={x} version={opened} onClose={() => setOpened(null)} />
      ) : null}
      {deleting ? (
        <DeleteDialog x={x} onClose={() => setDeleting(false)} />
      ) : null}
    </li>
  );
}

function ImportButton() {
  const { t } = useTranslation("workshop");
  const importer = useImportExtension();
  const [bundle, setBundle] = useState<Bundle | null>(null);
  const [slug, setSlug] = useState("");
  return (
    <>
      <label
        className="inline-flex cursor-pointer items-center gap-1 text-ui text-fg-muted hover:text-fg"
        title={t("extensions.importHint")}
      >
        <Upload className="size-4" />
        {t("extensions.import")}
        <input
          type="file"
          accept="application/json,.json"
          className="sr-only"
          onChange={async (e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (!file) return;
            try {
              const b = JSON.parse(await file.text()) as Bundle;
              setBundle(b);
              setSlug(b.slug);
            } catch {
              toast(t("extensions.manifestBad"), { tone: "danger" });
            }
          }}
        />
      </label>
      {bundle ? (
        <Dialog
          open
          onOpenChange={(o) => !o && setBundle(null)}
          title={t("extensions.import")}
          closeLabel={t("extensions.close")}
          footer={
            <Button
              variant="primary"
              disabled={!validSlug(slug)}
              loading={importer.isPending}
              onClick={() =>
                importer.mutate(
                  { bundle, ...(slug !== bundle.slug ? { slug } : {}) },
                  {
                    onSuccess: (x) => {
                      toast(t("extensions.imported", { name: x.title }));
                      setBundle(null);
                    },
                  },
                )
              }
            >
              {t("extensions.import")}
            </Button>
          }
        >
          <div className="flex flex-col gap-3">
            {importer.error ? (
              <Notice tone="danger">{importer.error.message}</Notice>
            ) : null}
            <p className="text-ui">
              {bundle.title} · v{bundle.version}
            </p>
            <p className="text-meta text-fg-muted">
              {t("extensions.importHint")}
            </p>
            <label className="flex flex-col gap-1 text-ui">
              <span className="font-medium">{t("extensions.importSlug")}</span>
              <Input
                value={slug}
                maxLength={16}
                className="font-mono"
                onChange={(e) => setSlug(e.target.value)}
              />
            </label>
          </div>
        </Dialog>
      ) : null}
    </>
  );
}

function CreateDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation("workshop");
  const create = useCreateExtension();
  const [title, setTitle] = useState("");
  const [slug, setSlug] = useState("");
  const [touched, setTouched] = useState(false);
  const [description, setDescription] = useState("");
  const [bot, setBot] = useState("");
  const shown = touched ? slug : slugFrom(title);
  const ok = title.trim() !== "" && validSlug(shown);
  const submit = () => {
    if (!ok) return;
    create.mutate(
      {
        slug: shown,
        title: title.trim(),
        ...(description.trim() ? { description: description.trim() } : {}),
        ...(bot.trim() ? { bot_handle: bot.trim() } : {}),
      },
      { onSuccess: onClose },
    );
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("extensions.createTitle")}
      closeLabel={t("extensions.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("extensions.cancel")}
          </Button>
          <Button
            variant="primary"
            disabled={!ok}
            loading={create.isPending}
            onClick={submit}
          >
            {t("extensions.create")}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {create.error ? (
          <Notice tone="danger">{create.error.message}</Notice>
        ) : null}
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("extensions.title")}</span>
          <Input
            value={title}
            autoFocus
            maxLength={80}
            onChange={(e) => setTitle(e.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("extensions.slug")}</span>
          <Input
            value={shown}
            maxLength={16}
            className="font-mono"
            onChange={(e) => {
              setTouched(true);
              setSlug(e.target.value);
            }}
          />
          <span className="text-meta text-fg-muted">
            {t("extensions.slugHint", { slug: shown || "…" })}
          </span>
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("extensions.description")}</span>
          <Textarea
            value={description}
            maxLength={2000}
            onChange={(e) => setDescription(e.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("extensions.botHandle")}</span>
          <Input
            value={bot}
            maxLength={40}
            className="font-mono"
            placeholder={`x-${(shown || "slug").replaceAll("_", "-")}`}
            onChange={(e) => setBot(e.target.value)}
          />
          <span className="text-meta text-fg-muted">
            {t("extensions.botHandleHint", {
              slug: (shown || "slug").replaceAll("_", "-"),
            })}
          </span>
        </label>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

function VersionEditor({
  x,
  onClose,
  onSaved,
}: {
  x: Extension;
  onClose: () => void;
  onSaved: (version: number) => void;
}) {
  const { t } = useTranslation("workshop");
  const latest = x.versions[0]?.version ?? 0;
  const prev = useExtensionVersion(x.id, latest);
  const add = useAddVersion(x.id);
  const [manifest, setManifest] = useState<string | null>(null);
  const [source, setSource] = useState<string | null>(null);
  const shownManifest =
    manifest ??
    (prev.data
      ? JSON.stringify(prev.data.manifest, null, 2)
      : latest
        ? ""
        : manifestTemplate);
  const shownSource =
    source ?? (prev.data ? prev.data.source : latest ? "" : sourceTemplate);
  const parsed = parseObject(shownManifest);
  const submit = () => {
    if (!parsed) return;
    add.mutate(
      {
        manifest: parsed,
        ...(shownSource.trim() ? { source: shownSource } : {}),
      },
      {
        onSuccess: (v) => {
          toast(t("extensions.saved", { version: v.version }));
          onClose();
          onSaved(v.version);
        },
      },
    );
  };
  return (
    <Dialog
      open
      size="xl"
      onOpenChange={(o) => !o && onClose()}
      title={t("extensions.editorTitle", { name: x.title })}
      closeLabel={t("extensions.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("extensions.cancel")}
          </Button>
          <Button
            variant="primary"
            disabled={!parsed}
            loading={add.isPending}
            onClick={submit}
          >
            {t("extensions.save")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {add.error ? <Notice tone="danger">{add.error.message}</Notice> : null}
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("extensions.manifest")}</span>
          <Textarea
            value={shownManifest}
            spellCheck={false}
            className="min-h-48 font-mono text-meta"
            onChange={(e) => setManifest(e.target.value)}
          />
          <span
            className={
              parsed ? "text-meta text-fg-muted" : "text-meta text-danger"
            }
          >
            {parsed
              ? t("extensions.manifestHint")
              : t("extensions.manifestBad")}
          </span>
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("extensions.source")}</span>
          <Textarea
            value={shownSource}
            spellCheck={false}
            className="min-h-64 font-mono text-meta"
            onChange={(e) => setSource(e.target.value)}
          />
          <span className="text-meta text-fg-muted">
            {t("extensions.sourceHint")}
          </span>
        </label>
      </div>
    </Dialog>
  );
}

function DiffView({ diff }: { diff: ExtensionDiff }) {
  const { t } = useTranslation("workshop");
  if (diffIsEmpty(diff))
    return <p className="text-ui text-fg-muted">{t("extensions.diffNone")}</p>;
  const list = (xs: string[] | undefined) => (xs ?? []).join(", ");
  const risks = Object.entries(diff.raised_risks ?? {}).map(
    ([k, v]) =>
      `${k} → ${t(`connectors.risk.${v as "read" | "write" | "external"}`)}`,
  );
  return (
    <ul className="flex list-disc flex-col gap-1 pl-5 text-ui">
      {diff.new_connectors?.length ? (
        <li>
          {t("extensions.diffConnectors", { list: list(diff.new_connectors) })}
        </li>
      ) : null}
      {diff.new_egress?.length ? (
        <li>{t("extensions.diffEgress", { list: list(diff.new_egress) })}</li>
      ) : null}
      {diff.new_store?.length ? (
        <li>{t("extensions.diffStore", { list: list(diff.new_store) })}</li>
      ) : null}
      {diff.new_routines?.length ? (
        <li>
          {t("extensions.diffRoutines", { list: list(diff.new_routines) })}
        </li>
      ) : null}
      {diff.new_tools?.length ? (
        <li>{t("extensions.diffTools", { list: list(diff.new_tools) })}</li>
      ) : null}
      {risks.length ? (
        <li>{t("extensions.diffRisks", { list: risks.join(", ") })}</li>
      ) : null}
      {diff.code_changed ? <li>{t("extensions.diffCode")}</li> : null}
    </ul>
  );
}

function VersionDialog({
  x,
  version,
  onClose,
}: {
  x: Extension;
  version: number;
  onClose: () => void;
}) {
  const { t } = useTranslation("workshop");
  const v = useExtensionVersion(x.id, version);
  const approve = useApproveVersion(x.id);
  const rollback = useRollback(x.id);
  const test = useTestTool(x.id);
  const summary = x.versions.find((s) => s.version === version);
  const tools = summary?.tools ?? [];
  const [tool, setTool] = useState(tools[0]?.name ?? "");
  const [input, setInput] = useState("{}");
  const parsedInput = parseObject(input);
  const fail = (e: Error) => toast(e.message, { tone: "danger" });
  const isLive = x.live_version === version;
  return (
    <Dialog
      open
      size="xl"
      onOpenChange={(o) => !o && onClose()}
      title={t("extensions.versionTitle", { name: x.title, version })}
      closeLabel={t("extensions.close")}
    >
      {v.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : v.isError ? (
        <p className="text-ui text-danger">{v.error.message}</p>
      ) : (
        <div className="flex flex-col gap-4">
          <section className="flex flex-col gap-2">
            <h3 className="font-semibold">{t("extensions.diff")}</h3>
            <DiffView diff={v.data.diff} />
            <div className="flex flex-wrap gap-2">
              {!isLive ? (
                <Button
                  variant="primary"
                  loading={approve.isPending}
                  onClick={() =>
                    approve.mutate(version, {
                      onSuccess: onClose,
                      onError: fail,
                    })
                  }
                >
                  {t("extensions.approve")}
                </Button>
              ) : null}
              {!isLive && summary?.approved ? (
                <Button
                  variant="secondary"
                  loading={rollback.isPending}
                  onClick={() =>
                    rollback.mutate(version, {
                      onSuccess: onClose,
                      onError: fail,
                    })
                  }
                >
                  {t("extensions.rollback")}
                </Button>
              ) : null}
            </div>
          </section>
          <section className="flex flex-col gap-2">
            <h3 className="font-semibold">{t("extensions.test")}</h3>
            <p className="text-meta text-fg-muted">
              {t("extensions.testHint")}
            </p>
            <div className="flex flex-wrap items-end gap-2">
              <label className="flex flex-col gap-1 text-ui">
                <span className="font-medium">{t("extensions.tool")}</span>
                <Select value={tool} onChange={(e) => setTool(e.target.value)}>
                  {tools.map((tl) => (
                    <option key={tl.name} value={tl.name}>
                      {tl.name}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="flex min-w-48 flex-1 flex-col gap-1 text-ui">
                <span className="font-medium">{t("extensions.input")}</span>
                <Input
                  value={input}
                  className="font-mono"
                  onChange={(e) => setInput(e.target.value)}
                />
              </label>
              <Button
                variant="secondary"
                disabled={!tool || !parsedInput}
                loading={test.isPending}
                onClick={() =>
                  parsedInput &&
                  test.mutate(
                    { version, tool, input: parsedInput },
                    { onError: fail },
                  )
                }
              >
                {t("extensions.run")}
              </Button>
            </div>
            {test.data ? (
              <div className="flex flex-col gap-1">
                <span className="text-meta font-medium">
                  {t("extensions.output")}
                </span>
                <pre
                  className={`max-h-48 overflow-auto rounded-md border border-border p-2 font-mono text-meta whitespace-pre-wrap ${test.data.is_error ? "text-danger" : ""}`}
                >
                  {test.data.output}
                </pre>
                {test.data.logs.length ? (
                  <>
                    <span className="text-meta font-medium">
                      {t("extensions.logs")}
                    </span>
                    <pre className="max-h-32 overflow-auto rounded-md border border-border p-2 font-mono text-meta whitespace-pre-wrap">
                      {test.data.logs.join("\n")}
                    </pre>
                  </>
                ) : null}
              </div>
            ) : null}
          </section>
          <section className="flex flex-col gap-2">
            <h3 className="font-semibold">{t("extensions.manifest")}</h3>
            <pre className="max-h-64 overflow-auto rounded-md border border-border p-2 font-mono text-meta">
              {JSON.stringify(v.data.manifest, null, 2)}
            </pre>
            {v.data.source ? (
              <>
                <h3 className="font-semibold">{t("extensions.source")}</h3>
                <pre className="max-h-64 overflow-auto rounded-md border border-border p-2 font-mono text-meta">
                  {v.data.source}
                </pre>
              </>
            ) : null}
          </section>
        </div>
      )}
    </Dialog>
  );
}

function DeleteDialog({ x, onClose }: { x: Extension; onClose: () => void }) {
  const { t } = useTranslation("workshop");
  const del = useDeleteExtension();
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("extensions.deleteTitle", { name: x.title })}
      closeLabel={t("extensions.close")}
      footer={
        <Button
          variant="danger"
          loading={del.isPending}
          onClick={() =>
            del.mutate(x.id, {
              onSuccess: onClose,
              onError: (e) => toast(e.message, { tone: "danger" }),
            })
          }
        >
          {t("extensions.delete")}
        </Button>
      }
    >
      <p>{t("extensions.deleteBody")}</p>
    </Dialog>
  );
}
