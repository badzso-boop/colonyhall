// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from "react";
import { useTranslation } from "react-i18next";
import { CircleStop, ExternalLink, MonitorPlay, RotateCw, X } from "lucide-react";
import { useActors } from "@/features/identity";
import { useLocalStorage } from "@/shared/hooks";
import { cn, sanitizeUrl } from "@/shared/lib";
import { Badge, Button, Dialog, EmptyState, IconButton, toast } from "@/shared/ui";
import { usePreviews, useRestartPreview, useStopPreview } from "../api";
import { usePreviewParam } from "../pane";
import { blocker, clampWidth, DEFAULT_WIDTH, dragWidth, MIN_WIDTH, WIDTH_KEY, type Preview } from "../model";

const stateTone = { requested: "info", running: "success", stopped: "neutral", failed: "danger" } as const;

/**
 * The embedded preview (round 57): a toolbar (name, agent, state; refresh,
 * new tab, stop, close) over a sandboxed frame of the proxy address. The
 * frame's origin is the preview listener's, never the app's, so the
 * agent's scripts cannot act with the viewer's session.
 */
export function LivePreviewPane({ previewId, onClose }: { previewId: string; onClose: () => void }) {
  const { t } = useTranslation("previews");
  const previews = usePreviews();
  const actors = useActors();
  const stop = useStopPreview();
  const restart = useRestartPreview();
  const [, open] = usePreviewParam();
  const [reload, setReload] = useState(0);
  const [confirm, setConfirm] = useState(false);
  const p: Preview | undefined = previews.data?.find((x) => x.id === previewId);
  const src = p?.proxy_url ? sanitizeUrl(p.proxy_url) : null;
  const shown = p ? blocker(p) : null;
  return (
    <section aria-label={t("title")} className="flex h-full min-h-0 flex-col bg-surface">
      <div className="flex h-10 shrink-0 items-center gap-1.5 border-b border-border px-2">
        <MonitorPlay aria-hidden className="size-4 shrink-0 text-fg-subtle" />
        <span className="truncate text-ui font-medium">{p?.name ?? t("title")}</span>
        {p ? <span className="truncate font-mono text-meta text-fg-muted">@{actors.data?.get(p.agent_id)?.handle ?? "?"}</span> : null}
        {p ? <Badge tone={stateTone[p.state]}>{t(`states.${p.state}`)}</Badge> : null}
        <span className="flex-1" />
        <IconButton size="sm" label={t("refresh")} icon={<RotateCw className="size-4" />} disabled={!src || shown !== null} onClick={() => setReload((n) => n + 1)} />
        {src && shown === null ? (
          <a
            href={src}
            target="_blank"
            rel="noopener noreferrer"
            aria-label={t("openNewTab")}
            title={t("openNewTab")}
            className="inline-grid size-7 place-items-center rounded-md text-fg-muted hover:bg-surface-sunken hover:text-fg"
          >
            <ExternalLink aria-hidden className="size-4" />
          </a>
        ) : null}
        {p && (p.state === "running" || p.state === "requested") ? (
          <IconButton size="sm" label={t("stop")} icon={<CircleStop className="size-4 text-danger" />} onClick={() => setConfirm(true)} />
        ) : null}
        {p && (p.state === "stopped" || p.state === "failed") ? (
          <Button
            size="sm"
            icon={<RotateCw className="size-4" />}
            title={t("restartHint", { name: p.name })}
            loading={restart.isPending}
            onClick={() =>
              restart.mutate(p.id, {
                onSuccess: (np) => {
                  toast(t("restarting"));
                  open(np.id);
                },
                onError: () => toast(t("restartFailed"), { tone: "danger" }),
              })
            }
          >
            {t("restart")}
          </Button>
        ) : null}
        <IconButton size="sm" label={t("close")} icon={<X className="size-4" />} onClick={onClose} />
      </div>
      <div className="relative min-h-0 flex-1 bg-bg">
        {!p ? (
          previews.isSuccess ? <EmptyState title={t("notFound")} /> : null
        ) : shown !== null || !src ? (
          <EmptyState title={t(shown ?? "noProxy", { state: t(`states.${p.state}`) })} />
        ) : (
          <iframe
            key={reload}
            title={t("frame", { name: p.name })}
            src={src}
            className="size-full border-0 bg-bg"
            sandbox="allow-scripts allow-forms allow-same-origin allow-popups allow-modals"
            referrerPolicy="no-referrer"
          />
        )}
      </div>
      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("confirmTitle", { name: p?.name ?? "" })}
        description={t("confirmBody")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(false)}>{t("cancel")}</Button>
            <Button
              variant="danger"
              loading={stop.isPending}
              onClick={() =>
                p &&
                stop.mutate(p.id, {
                  onSuccess: () => toast(t("stopped")),
                  onError: () => toast(t("stopFailed"), { tone: "danger" }),
                  onSettled: () => setConfirm(false),
                })
              }
            >
              {t("stop")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </section>
  );
}

/**
 * The split view's column (round 57): the preview on the right of the
 * conversation, its width dragged on the divider (or with the arrow keys)
 * between 320 px and 80% of the window, kept in localStorage. On a narrow
 * screen it is a full-screen sheet instead.
 */
export function PreviewColumn({ previewId, narrow, onClose }: { previewId: string; narrow: boolean; onClose: () => void }) {
  const { t } = useTranslation("previews");
  const [stored, setStored] = useLocalStorage<number>(WIDTH_KEY, DEFAULT_WIDTH);
  const [width, setWidth] = useState(() => clampWidth(stored, window.innerWidth));
  const [dragging, setDragging] = useState(false);
  const start = useRef<{ x: number; width: number } | null>(null);

  useEffect(() => {
    const fit = () => setWidth((w) => clampWidth(w, window.innerWidth));
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, []);

  if (narrow) {
    return (
      <div role="dialog" aria-modal="true" aria-label={t("title")} className="fixed inset-0 z-(--z-overlay) bg-surface">
        <LivePreviewPane previewId={previewId} onClose={onClose} />
      </div>
    );
  }

  const down = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    e.preventDefault();
    // Pointer capture keeps the moves coming while the pointer is over the frame.
    e.currentTarget.setPointerCapture?.(e.pointerId);
    start.current = { x: e.clientX, width };
    setDragging(true);
  };
  const move = (e: PointerEvent<HTMLDivElement>) => {
    if (!start.current) return;
    setWidth(dragWidth(start.current.width, start.current.x, e.clientX, window.innerWidth));
  };
  const up = (e: PointerEvent<HTMLDivElement>) => {
    if (!start.current) return;
    const next = dragWidth(start.current.width, start.current.x, e.clientX, window.innerWidth);
    start.current = null;
    setDragging(false);
    setWidth(next);
    setStored(next);
  };
  const key = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = e.shiftKey ? 64 : 16;
    const delta = e.key === "ArrowLeft" ? step : e.key === "ArrowRight" ? -step : 0;
    if (!delta) return;
    e.preventDefault();
    const next = clampWidth(width + delta, window.innerWidth);
    setWidth(next);
    setStored(next);
  };

  return (
    <div className="relative flex shrink-0 border-l border-border" style={{ width }}>
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label={t("resize")}
        aria-valuemin={MIN_WIDTH}
        aria-valuemax={Math.floor(window.innerWidth * 0.8)}
        aria-valuenow={width}
        tabIndex={0}
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={up}
        onKeyDown={key}
        className={cn(
          "absolute inset-y-0 -left-1.5 z-(--z-pane) w-3 cursor-col-resize touch-none outline-none",
          "after:absolute after:inset-y-0 after:left-1/2 after:w-0.5 after:-translate-x-1/2 after:transition-colors",
          "hover:after:bg-accent focus-visible:after:bg-accent",
          dragging && "after:bg-accent",
        )}
      />
      {/* While dragging, the frame must not swallow the pointer. */}
      <div className={cn("min-w-0 flex-1", dragging && "pointer-events-none select-none")}>
        <LivePreviewPane previewId={previewId} onClose={onClose} />
      </div>
    </div>
  );
}
