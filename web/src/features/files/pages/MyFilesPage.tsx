import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Copy, Download, FileText, FolderOpen, Image as ImageIcon, MessageSquare, Trash2 } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { cn, formatBytes, fullTime } from "@/shared/lib";
import { Button, Checkbox, Dialog, EmptyState, Notice, PageHeader, Select, Skeleton, toast } from "@/shared/ui";
import { useDeleteFiles, useMyFiles, type FileKind, type MyFile, type Uploader } from "../storage";
import { channelOptions, messageLink } from "../model";

const kinds: FileKind[] = ["image", "document", "other"];

/** Fájljaim (round 54): the files I uploaded, and my agents' files, to look through and delete. */
export default function MyFilesPage() {
  const { t, i18n } = useTranslation("files");
  const locale = localeOf(i18n.language);
  const [uploader, setUploader] = useState<Uploader>("me");
  const [channel, setChannel] = useState("");
  const [kind, setKind] = useState<FileKind | "">("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [confirm, setConfirm] = useState(false);
  const list = useMyFiles(uploader, channel, kind);
  const del = useDeleteFiles();
  const rows = useMemo(() => list.data?.pages.flatMap((p) => p.files) ?? [], [list.data]);
  // The channel filter offers the channels of the listed files ("All channels" lists them all again).
  const options = useMemo(() => channelOptions(rows, new Map(), t("my.dm")), [rows, t]);

  const chosen = rows.filter((f) => selected.has(f.id));
  const chosenBytes = chosen.reduce((n, f) => n + f.size, 0);
  const toggle = (id: string) =>
    setSelected((s) => {
      const next = new Set(s);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  const switchTab = (u: Uploader) => {
    setUploader(u);
    setSelected(new Set());
  };
  const remove = () =>
    del.mutate([...selected], {
      onSuccess: (failed) => {
        setConfirm(false);
        setSelected(new Set(failed));
        const done = selected.size - failed.length;
        if (done) toast(t("my.deleted", { count: done }));
        if (failed.length) toast(t("my.deleteFailed", { count: failed.length }), { tone: "danger" });
      },
    });

  return (
    <div className="flex h-full flex-col">
      <PageHeader icon={<FolderOpen className="size-5" />} title={t("my.title")} />
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <div role="tablist" aria-label={t("my.title")} className="flex gap-1 border-b border-border">
          {(["me", "agents"] as const).map((u) => (
            <button
              key={u}
              type="button"
              role="tab"
              aria-selected={uploader === u}
              onClick={() => switchTab(u)}
              className={cn("-mb-px border-b-2 px-3 py-1.5 text-ui", uploader === u ? "border-accent font-semibold text-fg" : "border-transparent text-fg-muted hover:text-fg")}
            >
              {t(u === "me" ? "my.mine" : "my.agents")}
            </button>
          ))}
        </div>
        {uploader === "agents" ? <p className="text-meta text-fg-muted">{t("my.agentsHint")}</p> : null}
        <div className="flex flex-wrap items-center gap-2">
          <Select aria-label={t("my.channel")} value={channel} onChange={(e) => setChannel(e.target.value)} className="w-48">
            <option value="">{t("my.allChannels")}</option>
            {[...options].map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </Select>
          <Select aria-label={t("my.kind")} value={kind} onChange={(e) => setKind(e.target.value as FileKind | "")} className="w-40">
            <option value="">{t("my.allKinds")}</option>
            {kinds.map((k) => (
              <option key={k} value={k}>
                {t(`my.kinds.${k}`)}
              </option>
            ))}
          </Select>
          <span className="flex-1" />
          {selected.size ? (
            <span className="text-meta text-fg-muted tabular">{t("my.selected", { count: selected.size, size: formatBytes(chosenBytes, locale) })}</span>
          ) : null}
          <Button size="sm" variant="danger" icon={<Trash2 className="size-3.5" />} disabled={!selected.size} onClick={() => setConfirm(true)}>
            {t("my.delete")}
          </Button>
        </div>

        {list.isPending ? <Skeleton className="h-10 w-full" /> : null}
        {list.error ? <Notice tone="danger">{t("errors.generic")}</Notice> : null}
        {!list.isPending && !rows.length ? (
          <EmptyState icon={<FolderOpen className="size-6" />} title={t(uploader === "me" ? "my.empty" : "my.emptyAgents")} />
        ) : null}
        {rows.length ? (
          <ul className="flex flex-col divide-y divide-border rounded-lg border border-border bg-surface">
            {rows.map((f) => (
              <FileRow key={f.id} file={f} checked={selected.has(f.id)} onToggle={() => toggle(f.id)} locale={locale} />
            ))}
          </ul>
        ) : null}
        {list.hasNextPage ? (
          <Button size="sm" variant="secondary" className="self-center" disabled={list.isFetchingNextPage} onClick={() => void list.fetchNextPage()}>
            {t("my.more")}
          </Button>
        ) : null}
      </div>

      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("my.confirmTitle", { count: selected.size })}
        description={t("my.confirmBody")}
        closeLabel={t("my.cancel")}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirm(false)}>
              {t("my.cancel")}
            </Button>
            <Button variant="danger" disabled={del.isPending} onClick={remove}>
              {t("my.confirmDelete", { count: selected.size })}
            </Button>
          </>
        }
      >
        <ul className="flex flex-col gap-1 text-ui">
          {chosen.map((f) => (
            <li key={f.id} className="flex justify-between gap-3">
              <span className="truncate">{f.name}</span>
              <span className="tabular text-fg-subtle">{formatBytes(f.size, locale)}</span>
            </li>
          ))}
        </ul>
        {chosen.some((f) => f.shared_count > 0) ? <p className="mt-2 text-meta text-fg-muted">{t("my.sharedNote")}</p> : null}
      </Dialog>
    </div>
  );
}

function FileRow({ file: f, checked, onToggle, locale }: { file: MyFile; checked: boolean; onToggle: () => void; locale: string }) {
  const { t } = useTranslation("files");
  const Icon = f.mime.startsWith("image/") ? ImageIcon : FileText;
  const link = messageLink(f);
  return (
    <li className="flex items-center gap-3 px-3 py-2">
      <Checkbox checked={checked} onCheckedChange={onToggle} aria-label={t("my.select", { name: f.name })} />
      <Icon aria-hidden className="size-4 shrink-0 text-fg-subtle" />
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="truncate text-ui font-medium">{f.name}</span>
        <span className="flex flex-wrap gap-x-2 text-meta text-fg-subtle">
          <span className="tabular">{formatBytes(f.size, locale)}</span>
          <span>{f.channel ? (f.channel.kind === "dm" ? t("my.dm") : `#${f.channel.name}`) : t("my.noChannel")}</span>
          <span>{fullTime(f.created_at, locale)}</span>
          {f.shared_count > 0 ? (
            <span className="inline-flex items-center gap-1">
              <Copy aria-hidden className="size-3" />
              {t("my.shared", { count: f.shared_count })}
            </span>
          ) : null}
        </span>
      </div>
      {link ? (
        <Link to={link} className="inline-flex items-center gap-1 text-meta text-accent-text hover:underline">
          <MessageSquare aria-hidden className="size-3.5" />
          <span className="max-sm:sr-only">{t("my.openMessage")}</span>
        </Link>
      ) : null}
      <a href={`/api/v1/files/${f.id}/content`} download={f.name} aria-label={t("download")} className="rounded-sm p-1 text-fg-subtle hover:text-fg">
        <Download aria-hidden className="size-4" />
      </a>
    </li>
  );
}
