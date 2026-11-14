// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Copy, Square, SquareTerminal } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { relativeTime } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Badge, Button, Dialog, EmptyState, Skeleton, toast } from "@/shared/ui";
import { useAgentChannel, useAgentOverview } from "@/features/agents";
import { useActors, useSession } from "@/features/identity";
import { relayError, useKillTerminal, useTerminalOutput, useTerminals, type Terminal } from "../api";
import { formatDuration, sshAttach, terminalBadge } from "../model";

function copy(text: string, done: string) {
  void navigator.clipboard?.writeText(text).then(() => toast(done));
}

function StatusBadge({ term }: { term: Terminal }) {
  const { t } = useTranslation("terminals");
  const b = terminalBadge(term);
  return <Badge tone={b.tone}>{t(b.key, { code: b.code })}</Badge>;
}

function RelayError({ error, onRetry }: { error: unknown; onRetry: () => void }) {
  const { t } = useTranslation("terminals");
  const code = relayError(error, (error as { code?: string } | null)?.code);
  return (
    <EmptyState
      icon={<SquareTerminal className="size-8" />}
      title={t(`errors.${code}`, { defaultValue: t("errors.generic") })}
      action={<Button onClick={onRetry}>{t("retry")}</Button>}
    />
  );
}

function TerminalList({ agentId, channelId }: { agentId: string; channelId: string }) {
  const { t, i18n } = useTranslation("terminals");
  const locale = localeOf(i18n.language);
  const [, dispatch] = usePanes();
  const list = useTerminals(agentId, channelId);
  if (list.isError) return <RelayError error={list.error} onRetry={() => void list.refetch()} />;
  if (!list.data) return <Skeleton className="m-4 h-5 w-2/3" />;
  if (!list.data.length) return <EmptyState icon={<SquareTerminal className="size-8" />} title={t("empty")} />;
  return (
    <ul className="flex flex-col divide-y divide-border">
      {list.data.map((term) => (
        <li key={term.name}>
          <button
            type="button"
            onClick={() => dispatch({ type: "openWork", tab: { kind: "terminal", agentId, ref: term.name } })}
            className="flex w-full flex-col gap-1 px-4 py-2.5 text-left hover:bg-surface-sunken"
          >
            <span className="flex items-center gap-2">
              <span className="font-mono text-ui font-semibold">{term.name}</span>
              <StatusBadge term={term} />
              <span className="tabular ml-auto text-meta text-fg-subtle">{formatDuration(term.duration_ms)}</span>
            </span>
            <code className="truncate font-mono text-meta text-fg-muted">{term.command}</code>
            <span className="text-meta text-fg-subtle">{t("started", { when: relativeTime(term.started_at, new Date(), locale) })}</span>
          </button>
          {term.thread_root_id ? (
            <button type="button" onClick={() => dispatch({ type: "openThread", rootId: term.thread_root_id! })} className="mb-2 ml-4 text-meta text-accent-text hover:underline">
              {t("thread")}
            </button>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function TerminalOutput({ agentId, channelId, name }: { agentId: string; channelId: string; name: string }) {
  const { t } = useTranslation("terminals");
  const list = useTerminals(agentId, channelId);
  const term = list.data?.find((x) => x.name === name);
  const running = term?.status === "running";
  const out = useTerminalOutput(agentId, channelId, name, running);
  const kill = useKillTerminal(agentId, channelId);
  const overview = useAgentOverview(agentId);
  const actors = useActors();
  const session = useSession();
  const admin = session.data?.actor.role === "owner" || session.data?.actor.role === "admin";
  const handle = actors.data?.get(agentId)?.handle ?? "";
  const host = overview.data?.assignment?.host_name ?? "";
  const scroller = useRef<HTMLPreElement>(null);
  const follow = useRef(true);
  const [paused, setPaused] = useState(false);
  const [confirm, setConfirm] = useState(false);

  useEffect(() => {
    const el = scroller.current;
    if (el && follow.current) el.scrollTop = el.scrollHeight;
  }, [out.data?.output]);

  if (out.isError) return <RelayError error={out.error} onRetry={() => void out.refetch()} />;
  const ssh = sshAttach(handle, name);
  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2">
        <span className="font-mono text-ui font-semibold">{name}</span>
        {term ? <StatusBadge term={term} /> : null}
        {term ? <span className="tabular text-meta text-fg-subtle">{formatDuration(term.duration_ms)}</span> : null}
        {host ? <span className="text-meta text-fg-muted">· {host}</span> : null}
        <span className="ml-auto flex gap-1">
          <Button size="sm" variant="ghost" icon={<Copy className="size-3.5" />} onClick={() => copy(out.data?.output ?? "", t("copied"))}>
            {t("copyOutput")}
          </Button>
          {admin && running ? (
            <Button size="sm" variant="danger" icon={<Square className="size-3.5" />} onClick={() => setConfirm(true)}>
              {t("stop")}
            </Button>
          ) : null}
        </span>
      </header>
      {term ? <code className="border-b border-border bg-surface-sunken px-4 py-1.5 font-mono text-meta text-fg-muted">$ {term.command}</code> : null}
      <div className="relative min-h-0 flex-1 bg-terminal-bg">
        {!out.data ? (
          <Skeleton className="m-4 h-4 w-1/2" />
        ) : (
          <pre
            ref={scroller}
            onScroll={(e) => {
              const el = e.currentTarget;
              follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
              setPaused(!follow.current);
            }}
            className="h-full overflow-auto px-4 py-3 font-mono text-[12.5px] leading-[1.45] break-all whitespace-pre-wrap text-terminal-fg"
          >
            {out.data.truncated ? <span className="block pb-2 opacity-60">{t("truncated")}</span> : null}
            {out.data.output}
          </pre>
        )}
        {paused ? (
          <button
            type="button"
            onClick={() => {
              follow.current = true;
              setPaused(false);
              const el = scroller.current;
              if (el) el.scrollTop = el.scrollHeight;
            }}
            className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full bg-accent px-3 py-1 text-meta font-semibold text-accent-fg shadow-overlay"
          >
            {t("jumpEnd")}
          </button>
        ) : null}
      </div>
      <footer className="flex items-center gap-2 border-t border-border px-4 py-2 text-meta text-fg-muted">
        <span className="shrink-0">{t("ssh", { host: host || "?" })}</span>
        <code className="min-w-0 flex-1 truncate font-mono">{ssh}</code>
        <Button size="sm" variant="ghost" icon={<Copy className="size-3.5" />} onClick={() => copy(ssh, t("copied"))}>
          {t("copySsh")}
        </Button>
      </footer>
      <Dialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("stopTitle", { name })}
        description={t("stopBody")}
        closeLabel={t("cancel")}
        footer={
          <>
            <Button onClick={() => setConfirm(false)}>{t("cancel")}</Button>
            <Button
              variant="danger"
              loading={kill.isPending}
              onClick={() =>
                kill.mutate(name, {
                  onSuccess: () => toast(t("stopped")),
                  onSettled: () => setConfirm(false),
                })
              }
            >
              {t("stop")}
            </Button>
          </>
        }
      >
        <code className="font-mono text-meta">{term?.command}</code>
      </Dialog>
    </div>
  );
}

/**
 * The terminal work pane (round 31, part 6): the agent's terminals,
 * or one terminal's output, read-only.
 */
export function TerminalPane({ agentId, name }: { agentId: string; name: string }) {
  const { t } = useTranslation("terminals");
  const { channelId, pending } = useAgentChannel(agentId);
  if (pending) return <Skeleton className="m-4 h-5 w-1/2" />;
  if (!channelId) return <EmptyState icon={<SquareTerminal className="size-8" />} title={t("noChannel")} />;
  return name ? <TerminalOutput agentId={agentId} channelId={channelId} name={name} /> : <TerminalList agentId={agentId} channelId={channelId} />;
}
