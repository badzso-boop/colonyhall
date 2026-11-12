import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft, FolderOpen, RefreshCw, Search } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useWidth } from "@/shared/hooks";
import { Button, EmptyState, IconButton, Input, Skeleton } from "@/shared/ui";
import { useAgentChannel } from "@/features/agents";
import { useActors } from "@/features/identity";
import { useFileSearch, useGitStatus } from "../api";
import { gitMarks, parseRef } from "../model";
import { FileTree } from "./FileTree";
import { FileViewer } from "./FileViewer";

function Browser({ agentId, channelId, initial }: { agentId: string; channelId: string; initial: string }) {
  const { t } = useTranslation("files");
  const width = useWidth();
  const qc = useQueryClient();
  const actors = useActors();
  const handle = actors.data?.get(agentId)?.handle ?? "";
  const first = parseRef(initial);
  const [selected, setSelected] = useState<string | null>(first.path || null);
  const [lines] = useState(first);
  const [query, setQuery] = useState("");
  const [nonce, setNonce] = useState(0);
  const git = useGitStatus(agentId, channelId);
  const search = useFileSearch(agentId, channelId, query);
  const marks = useMemo(() => gitMarks((git.data ?? []).flatMap((r) => r.files ?? [])), [git.data]);
  const repo = git.data?.find((r) => r.branch);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["files", agentId] });
    setNonce((n) => n + 1);
  };

  const tree = (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        <Search aria-hidden className="size-4 shrink-0 text-fg-subtle" />
        <Input aria-label={t("search")} placeholder={t("search")} value={query} onChange={(e) => setQuery(e.target.value)} className="h-7" />
      </div>
      <div className="min-h-0 flex-1 overflow-auto p-1.5">
        {query.trim().length >= 2 ? (
          search.isPending ? (
            <Skeleton className="m-2 h-4 w-1/2" />
          ) : search.data?.results.length ? (
            <ul>
              {search.data.results.map((r) => (
                <li key={`${r.root}/${r.path}`}>
                  <button type="button" onClick={() => setSelected(r.path)} className="flex h-7 w-full items-center truncate rounded-sm px-2 text-left font-mono text-meta hover:bg-surface-sunken">
                    {r.path}
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <p className="p-2 text-meta text-fg-subtle">{t("noResults")}</p>
          )
        ) : (
          <FileTree agentId={agentId} channelId={channelId} letters={marks.letters} dirty={marks.dirty} selected={selected} onOpen={setSelected} />
        )}
      </div>
    </div>
  );

  const viewer = selected ? (
    <FileViewer
      key={`${selected}-${nonce}`}
      agentId={agentId}
      channelId={channelId}
      handle={handle}
      path={selected}
      start={selected === lines.path ? lines.start : undefined}
      end={selected === lines.path ? lines.end : undefined}
      nonce={nonce}
    />
  ) : (
    <EmptyState icon={<FolderOpen className="size-8" />} title={t("pick")} />
  );

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex items-center gap-2 border-b border-border px-3 py-1.5 text-meta text-fg-muted">
        <span className="font-mono">@{handle}</span>
        {repo?.branch ? <span className="font-mono">· {t("branch", { branch: repo.branch })}</span> : null}
        {repo && (repo.ahead || repo.behind) ? <span className="tabular">{t("aheadBehind", { ahead: repo.ahead ?? 0, behind: repo.behind ?? 0 })}</span> : null}
        <IconButton className="ml-auto" size="sm" label={t("refresh")} icon={<RefreshCw className="size-3.5" />} onClick={refresh} />
      </header>
      {width === "narrow" ? (
        selected ? (
          <div className="flex min-h-0 flex-1 flex-col">
            <Button variant="ghost" size="sm" icon={<ArrowLeft className="size-4" />} onClick={() => setSelected(null)} className="justify-start">
              {t("back")}
            </Button>
            <div className="min-h-0 flex-1">{viewer}</div>
          </div>
        ) : (
          <div className="min-h-0 flex-1">{tree}</div>
        )
      ) : (
        <div className="flex min-h-0 flex-1 flex-col @container">
          <div className="grid min-h-0 flex-1 grid-rows-[minmax(8rem,40%)_1fr] @[720px]:grid-cols-[minmax(14rem,32%)_1fr] @[720px]:grid-rows-1">
            <div className="min-h-0 border-b border-border @[720px]:border-r @[720px]:border-b-0">{tree}</div>
            <div className="min-h-0">{viewer}</div>
          </div>
        </div>
      )}
    </div>
  );
}

/** The file browser work pane (round 31, part 7), read-only. */
export function FileBrowserPane({ agentId, path }: { agentId: string; path: string }) {
  const { t } = useTranslation("files");
  const { channelId, pending } = useAgentChannel(agentId);
  if (pending) return <Skeleton className="m-4 h-5 w-1/2" />;
  if (!channelId) return <EmptyState icon={<FolderOpen className="size-8" />} title={t("noChannel")} />;
  return <Browser agentId={agentId} channelId={channelId} initial={path} />;
}
