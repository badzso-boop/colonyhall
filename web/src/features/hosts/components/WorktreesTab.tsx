import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ExternalLink, GitBranch, MonitorPlay } from "lucide-react";
import { useActors } from "@/features/identity";
import { localeOf } from "@/shared/i18n";
import { fullTime, relativeTime, sanitizeUrl } from "@/shared/lib";
import { Badge, Button, Dialog, EmptyState, toast } from "@/shared/ui";
import { usePreviews, useStopPreview, type Host, type Preview } from "../api";
import { Mono } from "./OverviewTab";

const stateTone = { requested: "info", running: "success", stopped: "neutral", failed: "danger" } as const;

/** Worktree-k és előnézetek: the worktrees the guard reported and the machine's previews. */
export function WorktreesTab({ host, now }: { host: Host; now: Date }) {
  const { t, i18n } = useTranslation("hosts");
  const locale = localeOf(i18n.language);
  const actors = useActors();
  const previews = usePreviews();
  const stop = useStopPreview();
  const [confirm, setConfirm] = useState<Preview | null>(null);
  const handle = (id: string) => {
    const a = host.agents.find((x) => x.agent_id === id);
    return `@${a?.handle ?? actors.data?.get(id)?.handle ?? id.slice(0, 8)}`;
  };
  const worktrees = host.report.worktrees ?? [];
  const mine = (previews.data ?? []).filter((p) => p.host_id === host.id);

  return (
    <div className="flex flex-col gap-6">
      <section>
        <h3 className="mb-2 flex items-center gap-2 text-title font-semibold">
          <GitBranch aria-hidden className="size-4 text-fg-subtle" />
          {t("worktrees.title")}
        </h3>
        {worktrees.length ? (
          <ul className="divide-y divide-border">
            {worktrees.map((w) => (
              <li key={`${w.agent_id}:${w.path}`} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-1.5 text-ui">
                <span className="font-mono">{handle(w.agent_id)}</span>
                <Mono>{w.path}</Mono>
                <Badge mono>{w.branch}</Badge>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-ui text-fg-muted">{t("worktrees.empty")}</p>
        )}
      </section>
      <section>
        <h3 className="mb-2 flex items-center gap-2 text-title font-semibold">
          <MonitorPlay aria-hidden className="size-4 text-fg-subtle" />
          {t("previews.title")}
        </h3>
        {previews.isError ? <p className="text-ui text-danger">{t("previews.error")}</p> : null}
        {mine.length ? (
          <ul className="divide-y divide-border">
            {mine.map((p) => {
              const url = p.url ? sanitizeUrl(p.url) : null;
              const live = p.state === "running" || p.state === "requested";
              return (
                <li key={p.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 py-1.5 text-ui">
                  <span className="font-mono">{handle(p.agent_id)}</span>
                  <span className="font-medium">{p.name}</span>
                  {url ? (
                    <a href={url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-accent-text hover:underline">
                      {p.url}
                      <ExternalLink aria-hidden className="size-3" />
                    </a>
                  ) : null}
                  <Badge tone={stateTone[p.state]}>{t(`previews.states.${p.state}`)}</Badge>
                  {live ? (
                    <span className="text-meta text-fg-muted" title={fullTime(p.expires_at, locale)}>
                      {t("previews.expires", { when: relativeTime(p.expires_at, now, locale) })}
                    </span>
                  ) : null}
                  {live ? (
                    <Button size="sm" variant="danger" className="ml-auto" onClick={() => setConfirm(p)}>
                      {t("previews.stop")}
                    </Button>
                  ) : null}
                </li>
              );
            })}
          </ul>
        ) : previews.isSuccess ? (
          <EmptyState title={t("previews.empty")} />
        ) : null}
      </section>
      <Dialog
        open={confirm !== null}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t("previews.confirmTitle", { name: confirm?.name ?? "" })}
        description={t("previews.confirmBody")}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirm(null)}>{t("cancel")}</Button>
            <Button
              variant="danger"
              loading={stop.isPending}
              onClick={() =>
                confirm &&
                stop.mutate(confirm.id, {
                  onSuccess: () => toast(t("previews.stopped")),
                  onError: () => toast(t("previews.stopFailed"), { tone: "danger" }),
                  onSettled: () => setConfirm(null),
                })
              }
            >
              {t("previews.stop")}
            </Button>
          </>
        }
      >
        {null}
      </Dialog>
    </div>
  );
}
