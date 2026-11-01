import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Archive, LoaderCircle, TriangleAlert } from "lucide-react";
import { Button, Dialog, Markdown, Sheet, Skeleton, Textarea, toast } from "@/shared/ui";
import { useCompactionStatus, useEditSummary, useRegenerate, useSummary } from "../api";
import { liveHere, previewLines } from "../model";
import { useLiveCompactions } from "../store";

type Attachment = { id: string; name: string; mime: string };

/** The summary file of a compaction message: the markdown attachment, else the first. */
export function summaryFile(attachments: Attachment[] | undefined): Attachment | undefined {
  const list = attachments ?? [];
  return list.find((a) => a.name.endsWith(".md") || a.mime === "text/markdown") ?? list[0];
}

function EditDialog({ compactionId, text, open, onOpenChange }: { compactionId: string; text: string; open: boolean; onOpenChange: (o: boolean) => void }) {
  const { t } = useTranslation("compaction");
  const [draft, setDraft] = useState(text);
  const edit = useEditSummary(compactionId);
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      size="xl"
      title={t("edit.title")}
      description={t("edit.hint")}
      closeLabel={t("common:close")}
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button
            variant="primary"
            loading={edit.isPending}
            disabled={!draft.trim() || draft === text}
            onClick={() =>
              edit.mutate(draft, {
                onSuccess: () => {
                  toast(t("edit.saved"));
                  onOpenChange(false);
                },
                onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }),
              })
            }
          >
            {t("edit.save")}
          </Button>
        </>
      }
    >
      <Textarea aria-label={t("edit.label")} value={draft} onChange={(e) => setDraft(e.target.value)} className="h-[50vh] font-mono text-meta" />
    </Dialog>
  );
}

/**
 * The compaction system message (round 21): the summary's first lines,
 * "Összefoglaló megnyitása" (a Sheet with the markdown), and for humans who
 * can write here "Szerkesztés" and "Újragenerálás" (with confirmation).
 */
export function CompactionCard({ compactionId, attachments, canWrite }: { compactionId: string; attachments?: Attachment[]; canWrite: boolean }) {
  const { t } = useTranslation("compaction");
  const file = summaryFile(attachments);
  const summary = useSummary(file?.id ?? null);
  const regenerate = useRegenerate(compactionId);
  const [sheet, setSheet] = useState(false);
  const [editing, setEditing] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const lines = summary.data ? previewLines(summary.data) : [];

  return (
    <section aria-label={t("card.label")} className="mt-1 flex max-w-xl flex-col gap-1.5 rounded-lg border border-border bg-surface-raised p-2.5">
      <p className="flex items-center gap-2 text-ui font-semibold">
        <Archive aria-hidden className="size-4 text-fg-subtle" />
        {t("card.title")}
      </p>
      {!file ? null : summary.isPending ? (
        <Skeleton className="h-10" />
      ) : summary.isError ? (
        <p className="text-meta text-danger">{t("card.loadFailed")}</p>
      ) : (
        <div className="text-ui text-fg-muted">
          {lines.map((l, i) => (
            <p key={i} className="truncate">
              {l}
            </p>
          ))}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" variant="secondary" disabled={!summary.data} onClick={() => setSheet(true)}>
          {t("card.open")}
        </Button>
        {canWrite ? (
          <>
            <Button size="sm" variant="ghost" disabled={!summary.data} onClick={() => setEditing(true)}>
              {t("card.edit")}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setConfirm(true)}>
              {t("card.regenerate")}
            </Button>
          </>
        ) : null}
      </div>
      <Sheet open={sheet} onOpenChange={setSheet} title={t("card.title")} closeLabel={t("common:close")}>
        {summary.data ? <Markdown text={summary.data} copyLabel={t("copy")} copiedLabel={t("copied")} /> : null}
      </Sheet>
      {editing && summary.data !== undefined ? <EditDialog compactionId={compactionId} text={summary.data} open={editing} onOpenChange={setEditing} /> : null}
      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("regenerate.title")}
        description={t("regenerate.body")}
        closeLabel={t("common:close")}
        footer={
          <>
            <Button variant="ghost" onClick={() => setConfirm(false)}>
              {t("cancel")}
            </Button>
            <Button
              variant="danger"
              loading={regenerate.isPending}
              onClick={() =>
                regenerate.mutate(undefined, {
                  onSuccess: () => setConfirm(false),
                  onError: (e) => toast(e instanceof Error ? e.message : String(e), { tone: "danger" }),
                })
              }
            >
              {t("regenerate.confirm")}
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
 * The running and failed compactions of a place (the main stream or a
 * thread), at the end of the feed: a spinner card while the scribe works,
 * an error card (humans) when it failed.
 */
export function CompactionLive({ channelId, threadRootId = null, human }: { channelId: string; threadRootId?: string | null; human: boolean }) {
  const { t } = useTranslation("compaction");
  const live = useLiveCompactions((s) => s.live);
  const dismiss = useLiveCompactions((s) => s.dismiss);
  const status = useCompactionStatus(channelId, threadRootId, human);
  const here = liveHere(live, channelId, threadRootId);
  // Started before this page loaded: the counter knows, the live store does not.
  const pendingUnknown = status.data?.has_pending_compaction && !here.some((l) => l.state === "pending");
  const cards = [...here, ...(pendingUnknown ? [{ id: "status", state: "pending" as const, errorCode: null }] : [])];
  if (!cards.length) return null;
  return (
    <div className="flex flex-col gap-1.5 px-4 py-1.5">
      {cards.map((c) =>
        c.state === "pending" ? (
          <p key={c.id} role="status" className="flex max-w-xl items-center gap-2 rounded-lg border border-border bg-surface-raised p-2.5 text-ui text-fg-muted">
            <LoaderCircle aria-hidden className="size-4 motion-safe:animate-spin" />
            {t("live.pending")}
          </p>
        ) : human ? (
          <div key={c.id} role="alert" className="flex max-w-xl items-center gap-2 rounded-lg border border-danger bg-danger-soft p-2.5 text-ui text-fg">
            <TriangleAlert aria-hidden className="size-4 shrink-0 text-danger" />
            <span className="min-w-0 flex-1">
              {t("live.failed")} {c.errorCode ? <code className="font-mono text-meta">[{c.errorCode}]</code> : null}
            </span>
            <Button size="sm" variant="ghost" onClick={() => dismiss(c.id)}>
              {t("live.dismiss")}
            </Button>
          </div>
        ) : null,
      )}
    </div>
  );
}
