// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Link, Quote } from "lucide-react";
import { insertIntoComposer } from "@/shared/lib";
import { Button, EmptyState, Markdown, Skeleton, Switch, toast } from "@/shared/ui";
import { useFetchedFile, useFileText, type FetchRequest } from "../api";
import { LARGE_FILE, languageOf, lineReference, rangeFor } from "../model";
import { defineThemes, loadMonaco, type MonacoEditor } from "../monaco";

function MonacoView({ text, path, wrap, start, end, onSelect }: { text: string; path: string; wrap: boolean; start?: number; end?: number; onSelect: (a: number, b: number) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<MonacoEditor | null>(null);
  const [failed, setFailed] = useState(false);
  // The latest callback, so a new function on each render does not recreate the editor.
  const onSelectRef = useRef(onSelect);
  useEffect(() => {
    onSelectRef.current = onSelect;
  }, [onSelect]);

  useEffect(() => {
    let disposed = false;
    let sub: { dispose: () => void } | undefined;
    loadMonaco()
      .then((monaco) => {
        if (disposed || !host.current) return;
        const theme = defineThemes(monaco);
        const ed = monaco.editor.create(host.current, {
          readOnly: true, automaticLayout: true, minimap: { enabled: false }, scrollBeyondLastLine: false, theme,
          fontFamily: getComputedStyle(document.documentElement).getPropertyValue("--font-mono"), fontSize: 12.5,
        });
        ed.setModel(monaco.editor.createModel(text, languageOf(path)));
        sub = ed.onDidChangeCursorSelection((e) => onSelectRef.current(e.selection.startLineNumber, e.selection.endLineNumber));
        editor.current = ed;
        if (start) {
          ed.setSelection({ startLineNumber: start, startColumn: 1, endLineNumber: end ?? start, endColumn: 1e6 });
          ed.revealLinesInCenter(start, end ?? start);
        }
        // Follow the app theme.
        const mo = new MutationObserver(() => monaco.editor.setTheme(defineThemes(monaco)));
        mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
        const prev = sub;
        sub = { dispose: () => (prev.dispose(), mo.disconnect()) };
      })
      .catch(() => setFailed(true));
    return () => {
      disposed = true;
      sub?.dispose();
      editor.current?.dispose();
      editor.current = null;
    };
  }, [text, path, start, end]);

  useEffect(() => editor.current?.updateOptions({ wordWrap: wrap ? "on" : "off" }), [wrap]);

  if (failed) return <pre className="h-full overflow-auto p-3 font-mono text-meta whitespace-pre-wrap">{text}</pre>;
  return <div ref={host} className="h-full" />;
}

type StoredFile = NonNullable<FetchRequest["file"]>;

/** The state of a fetch: loading, an i18n EmptyState for a failure, or the stored file. */
export function FetchState({ fetched, children }: { fetched: { isPending: boolean; isError: boolean; data?: FetchRequest | undefined }; children: (file: StoredFile) => ReactNode }) {
  const { t } = useTranslation("files");
  if (fetched.isPending) return <p role="status" className="p-4 text-ui text-fg-muted">{t("loading")}</p>;
  if (fetched.data?.state === "failed" || fetched.isError) {
    const code = fetched.data?.error_code ?? "generic";
    return <EmptyState title={t(`errors.${code}`, { defaultValue: t("errors.generic") })} />;
  }
  const file = fetched.data?.state === "done" ? fetched.data.file : undefined;
  if (!file) return <p role="status" className="p-4 text-ui text-fg-muted">{t("loading")}</p>;
  return <>{children(file)}</>;
}

/** The selected file: Monaco (read-only), markdown rendered or raw, images, head/tail of large files. */
export function FileViewer({ agentId, channelId, handle, path, start, end, nonce }: { agentId: string; channelId: string; handle: string; path: string; start?: number; end?: number; nonce: number }) {
  const fetched = useFetchedFile(agentId, channelId, path, nonce);
  return <FetchState fetched={fetched}>{(file) => <FileContent file={file} handle={handle} path={path} start={start} end={end} />}</FetchState>;
}

/** A stored file's view (shared by the file browser and the message file Sheet). */
export function FileContent({ file, handle, path, start, end }: { file: StoredFile; handle: string; path: string; start?: number | undefined; end?: number | undefined }) {
  const { t } = useTranslation("files");
  const [part, setPart] = useState<"start" | "end">("start");
  const large = file.size > LARGE_FILE;
  const isImage = file.mime.startsWith("image/");
  const text = useFileText(!isImage ? file.id : null, large ? rangeFor(file.size, part) : null);
  const [rendered, setRendered] = useState(true);
  const [wrap, setWrap] = useState(false);
  const [sel, setSel] = useState<[number, number] | null>(start ? [start, end ?? start] : null);

  const ref = sel ? lineReference(handle, path, sel[0], sel[1]) : null;
  const isMarkdown = /\.md$/i.test(path);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-1.5 text-meta">
        <span className="min-w-0 flex-1 truncate font-mono text-fg-muted" title={path}>{path}</span>
        {isMarkdown ? (
          <Button size="sm" variant="ghost" onClick={() => setRendered(!rendered)}>{rendered ? t("raw") : t("rendered")}</Button>
        ) : null}
        {!isImage ? (
          <label className="flex items-center gap-1.5">
            <Switch checked={wrap} onCheckedChange={setWrap} /> {t("wrap")}
          </label>
        ) : null}
        <a href={`/api/v1/files/${file.id}/content`} download={file.name} className="text-accent-text hover:underline">{t("download")}</a>
      </div>
      {large ? (
        <div className="flex items-center gap-2 border-b border-border bg-warning-soft px-3 py-1 text-meta text-warning">
          {t("largeFile", { size: `${(file.size / 1048576).toFixed(1)} MB`, part: t(part) })}
          <Button size="sm" variant="ghost" onClick={() => setPart(part === "start" ? "end" : "start")}>{part === "start" ? t("end") : t("start")}</Button>
        </div>
      ) : null}
      {sel && ref ? (
        <div className="flex items-center gap-2 border-b border-border bg-surface-sunken px-3 py-1 text-meta">
          <code className="min-w-0 flex-1 truncate font-mono">{ref}</code>
          <Button size="sm" variant="secondary" icon={<Quote className="size-3.5" />} onClick={() => insertIntoComposer(ref)}>{t("insertRef")}</Button>
          <Button size="sm" variant="ghost" icon={<Link className="size-3.5" />} onClick={() => void navigator.clipboard?.writeText(ref).then(() => toast(t("copied")))}>{t("copyLink")}</Button>
        </div>
      ) : null}
      <div className="min-h-0 flex-1">
        {isImage ? (
          <div className="grid h-full place-items-center overflow-auto p-4">
            <img src={`/api/v1/files/${file.id}/content`} alt={file.name} className="max-h-full max-w-full object-contain" />
          </div>
        ) : !text.data ? (
          <Skeleton className="m-4 h-4 w-1/2" />
        ) : isMarkdown && rendered ? (
          <div className="h-full overflow-auto p-4"><Markdown text={text.data} copyLabel={t("copyLink")} copiedLabel={t("copied")} /></div>
        ) : (
          <MonacoView text={text.data} path={path} wrap={wrap} start={start} end={end} onSelect={(a, b) => setSel(a === b && a === 1 && !start ? null : [a, b])} />
        )}
      </div>
    </div>
  );
}

