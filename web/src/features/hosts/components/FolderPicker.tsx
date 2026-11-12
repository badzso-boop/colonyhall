import { useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Dialog as RDialog } from "radix-ui";
import { ChevronRight, Folder, FolderGit2, HardDrive, Lock, RotateCw, X } from "lucide-react";
import { localeOf } from "@/shared/i18n";
import { useWidth } from "@/shared/hooks";
import { cn, relativeTime } from "@/shared/lib";
import { Button, EmptyState, IconButton, Input, Skeleton, Switch, Tooltip } from "@/shared/ui";
import { dirsFailure, useHostDirs, type HostDirEntry } from "../api";
import { breadcrumb, covered, rootOf, selectable, setAccess, summary, toggle, visibleEntries, type Access, type PickMode, type Selection } from "../picker";

export type FolderPickerProps = {
  hostId: string;
  hostLastSeen?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: PickMode;
  /** Single mode: only git repositories (the worktree repo). */
  gitOnly?: boolean;
  /** The selection the caller already holds (pre-marked). */
  initial?: Selection | string;
  onConfirm?: (selection: Selection) => void;
  /** Browse mode: e.g. avatar stacks of the agents with a grant on a folder. */
  badge?: (path: string) => ReactNode;
  /** Admins may type a path when the guard is offline. */
  allowTyped?: boolean;
  title?: string;
};

function EntryIcon({ e }: { e: HostDirEntry }) {
  if (e.denied) return <Lock aria-hidden className="size-4 text-fg-subtle" />;
  if (e.is_git_repo) return <FolderGit2 aria-hidden className="size-4 text-info" />;
  return <Folder aria-hidden className={cn("size-4", e.heavy ? "text-fg-subtle" : "text-fg-muted")} />;
}

/** Access toggle of one selected folder (multi mode). */
function AccessToggle({ value, onChange, label }: { value: Access; onChange: (a: Access) => void; label: string }) {
  const { t } = useTranslation("hosts");
  return (
    <div role="radiogroup" aria-label={label} className="flex rounded-md border border-border p-0.5 text-meta">
      {(["read", "write"] as const).map((a) => (
        <button
          key={a}
          type="button"
          role="radio"
          aria-checked={value === a}
          onClick={(ev) => {
            ev.stopPropagation();
            onChange(a);
          }}
          className={cn("rounded-sm px-2 py-0.5", value === a ? "bg-accent-soft font-semibold text-fg" : "text-fg-muted")}
        >
          {t(`picker.${a}`)}
        </button>
      ))}
    </div>
  );
}

/**
 * The folder picker (round 33, part 7): the machine's guard.yaml
 * roots on the left, a folder list on the right, asked of the guard live.
 * Folders are never typed, except by an admin as a fallback.
 */
export function FolderPicker({ hostId, hostLastSeen, open, onOpenChange, mode, gitOnly, initial, onConfirm, badge, allowTyped, title }: FolderPickerProps) {
  const { t, i18n } = useTranslation("hosts");
  const locale = localeOf(i18n.language);
  const narrow = useWidth() === "narrow";
  const roots = useHostDirs(hostId, "", open);
  const rootPaths = useMemo(() => (roots.data?.entries ?? []).map((e) => e.path), [roots.data]);
  const [current, setCurrent] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [hidden, setHidden] = useState(false);
  const [typed, setTyped] = useState("");
  const [typing, setTyping] = useState(false);
  const [sel, setSel] = useState<Selection>(() =>
    initial instanceof Map ? new Map(initial) : typeof initial === "string" && initial ? new Map([[initial, "read" as Access]]) : new Map(),
  );
  const listRef = useRef<HTMLUListElement>(null);

  const folder = current ?? rootPaths[0] ?? "";
  const dirs = useHostDirs(hostId, folder, open && Boolean(folder));
  const root = rootOf(folder, rootPaths) ?? folder;
  const entries = visibleEntries(dirs.data?.entries ?? [], hidden, filter);
  const failure = roots.isError ? dirsFailure(roots.error) : dirs.isError ? dirsFailure(dirs.error) : null;

  const choose = (path: string) => {
    if (mode === "single") setSel(new Map([[path, "read"]]));
    else if (mode === "multi") setSel((s) => toggle(s, path));
  };
  const goUp = () => {
    const crumbs = breadcrumb(root, folder);
    if (crumbs.length > 1) setCurrent(crumbs[crumbs.length - 2]!.path);
  };
  const onListKey = (ev: KeyboardEvent<HTMLUListElement>) => {
    const items = [...(listRef.current?.querySelectorAll<HTMLElement>("[data-entry]") ?? [])];
    const at = items.indexOf(document.activeElement as HTMLElement);
    const path = (document.activeElement as HTMLElement | null)?.dataset.entry;
    const entry = entries.find((e) => e.path === path);
    if (ev.key === "ArrowDown" || ev.key === "ArrowUp") {
      ev.preventDefault();
      items[Math.max(0, Math.min(items.length - 1, at + (ev.key === "ArrowDown" ? 1 : -1)))]?.focus();
    } else if (ev.key === "Enter" && entry && !entry.denied && entry.has_children) {
      ev.preventDefault();
      setCurrent(entry.path);
    } else if (ev.key === " " && entry && selectable(entry, mode, gitOnly)) {
      ev.preventDefault();
      choose(entry.path);
    } else if (ev.key === "Backspace") {
      ev.preventDefault();
      goUp();
    }
  };
  const typedRoot = typed.trim() ? rootOf(typed.trim(), rootPaths) : undefined;
  const addTyped = () => {
    const p = typed.trim().replace(/\/+$/, "");
    if (!p.startsWith("/")) return;
    choose(p);
    setTyped("");
  };
  const nested = covered(sel);
  const sum = summary(sel);

  const body = (
    <div className="flex min-h-0 flex-1 max-md:flex-col">
      <nav aria-label={t("picker.roots")} className="flex w-56 shrink-0 flex-col gap-px overflow-y-auto border-r border-border p-2 max-md:w-full max-md:flex-row max-md:border-r-0 max-md:border-b">
        {roots.isPending ? <Skeleton className="h-8 w-full" /> : null}
        {(roots.data?.entries ?? []).map((r) => (
          <button
            key={r.path}
            type="button"
            aria-current={root === r.path ? "true" : undefined}
            onClick={() => setCurrent(r.path)}
            className={cn("flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-ui hover:bg-surface-sunken max-md:shrink-0", root === r.path && "bg-accent-soft font-semibold")}
          >
            <HardDrive aria-hidden className="size-4 shrink-0 text-fg-subtle" />
            <span className="truncate font-mono text-meta" title={r.path}>
              {r.path}
            </span>
          </button>
        ))}
      </nav>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        {failure ? (
          <div className="flex flex-1 flex-col items-center justify-center p-6">
            <EmptyState
              title={t(failure === "offline" ? "picker.offline" : failure === "timeout" ? "picker.timeout" : "picker.error")}
              body={failure === "offline" && hostLastSeen ? t("picker.lastSeen", { when: relativeTime(hostLastSeen, new Date(), locale) }) : undefined}
              action={
                <Button size="sm" icon={<RotateCw className="size-3.5" />} onClick={() => void (roots.isError ? roots.refetch() : dirs.refetch())}>
                  {t("picker.retry")}
                </Button>
              }
            />
          </div>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
              <nav aria-label={t("picker.path")} className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5 font-mono text-meta">
                {breadcrumb(root, folder).map((c, i, all) => (
                  <span key={c.path} className="flex items-center gap-0.5">
                    {i > 0 ? <ChevronRight aria-hidden className="size-3 text-fg-subtle" /> : null}
                    <button
                      type="button"
                      aria-current={i === all.length - 1 ? "page" : undefined}
                      onClick={() => setCurrent(c.path)}
                      className="rounded-sm px-1 hover:bg-surface-sunken"
                    >
                      {c.label}
                    </button>
                  </span>
                ))}
              </nav>
              <Input aria-label={t("picker.filter")} placeholder={t("picker.filter")} value={filter} onChange={(e) => setFilter(e.target.value)} className="h-7 w-40" />
              <label className="flex items-center gap-1.5 text-meta text-fg-muted">
                <Switch checked={hidden} onCheckedChange={setHidden} />
                {t("picker.hidden")}
              </label>
            </div>
            {dirs.data?.error_code ? <p className="px-3 py-2 text-ui text-danger">{t(`picker.errors.${dirs.data.error_code}`)}</p> : null}
            <ul ref={listRef} role="listbox" aria-label={t("picker.folders")} aria-multiselectable={mode === "multi"} onKeyDown={onListKey} className="min-h-0 flex-1 overflow-y-auto p-1">
              {dirs.isPending ? Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="my-1 h-7 w-full" />) : null}
              {entries.map((e) => {
                const can = selectable(e, mode, gitOnly);
                const access = sel.get(e.path);
                const row = (
                  <li
                    key={e.path}
                    role="option"
                    aria-selected={access !== undefined}
                    aria-disabled={!can && mode !== "browse"}
                    data-entry={e.path}
                    tabIndex={0}
                    onClick={() => can && choose(e.path)}
                    onDoubleClick={() => !e.denied && e.has_children && setCurrent(e.path)}
                    className={cn(
                      "flex h-9 items-center gap-2 rounded-md px-2 text-ui outline-none hover:bg-surface-sunken focus-visible:ring-2 focus-visible:ring-focus-ring",
                      access !== undefined && "bg-accent-soft",
                      (e.denied || e.heavy) && "text-fg-subtle",
                      !can && mode !== "browse" && "cursor-not-allowed",
                    )}
                  >
                    {mode === "multi" ? (
                      <input type="checkbox" tabIndex={-1} readOnly checked={access !== undefined} disabled={!can} aria-hidden className="accent-(--color-accent)" />
                    ) : null}
                    <EntryIcon e={e} />
                    <span className="min-w-0 flex-1 truncate">{e.name}</span>
                    {badge ? badge(e.path) : null}
                    {mode === "multi" && access ? <AccessToggle value={access} onChange={(a) => setSel((s) => setAccess(s, e.path, a))} label={t("picker.access", { name: e.name })} /> : null}
                    {!e.denied && e.has_children ? (
                      <IconButton
                        size="sm"
                        label={t("picker.open", { name: e.name })}
                        icon={<ChevronRight className="size-3.5" />}
                        onClick={(ev) => {
                          ev.stopPropagation();
                          setCurrent(e.path);
                        }}
                      />
                    ) : null}
                  </li>
                );
                return e.denied ? (
                  <Tooltip key={e.path} content={t("picker.denied")}>
                    {row}
                  </Tooltip>
                ) : (
                  row
                );
              })}
              {dirs.data && entries.length === 0 ? <li className="px-2 py-3 text-meta text-fg-subtle">{t("picker.empty")}</li> : null}
              {dirs.data?.truncated ? <li className="px-2 py-2 text-meta text-fg-subtle">{t("picker.truncated")}</li> : null}
            </ul>
          </>
        )}
        {allowTyped && mode !== "browse" ? (
          <div className="border-t border-border px-3 py-2">
            {typing ? (
              <div className="flex flex-wrap items-center gap-2">
                <Input aria-label={t("picker.typePath")} placeholder="/home/…" value={typed} onChange={(e) => setTyped(e.target.value)} className="h-7 flex-1 font-mono" />
                <Button size="sm" onClick={addTyped} disabled={!typed.trim().startsWith("/")}>
                  {t("picker.add")}
                </Button>
                {typed.trim() && !typedRoot ? <span className="text-meta text-warning">{t("picker.outsideRoots")}</span> : null}
              </div>
            ) : (
              <button type="button" onClick={() => setTyping(true)} className="text-meta text-fg-muted underline hover:text-fg">
                {t("picker.typePath")}
              </button>
            )}
          </div>
        ) : null}
      </div>
    </div>
  );

  const tray =
    mode === "browse" ? null : (
      <div className="flex flex-col gap-1 border-t border-border px-4 py-2">
        {mode === "multi" ? (
          <>
            <p className="text-meta font-semibold text-fg-muted">{t("picker.summary", { count: sum.total, read: sum.read, write: sum.write })}</p>
            <ul aria-label={t("picker.selected")} className="flex max-h-24 flex-wrap gap-1 overflow-y-auto">
              {[...sel.entries()].map(([p, a]) => {
                const outside = rootPaths.length > 0 && !rootOf(p, rootPaths);
                return (
                  <li key={p} className={cn("flex items-center gap-1 rounded-full bg-surface-sunken py-0.5 pr-0.5 pl-2 font-mono text-meta", outside && "bg-warning-soft text-warning")}>
                    {p} · {t(`picker.${a}`)}
                    <IconButton size="sm" label={t("picker.remove", { path: p })} icon={<X className="size-3" />} onClick={() => setSel((s) => toggle(s, p))} />
                  </li>
                );
              })}
            </ul>
            {nested.map((n) => (
              <p key={n.path} className="text-meta text-warning">
                {t("picker.nested", { path: n.path, by: n.by })}
              </p>
            ))}
          </>
        ) : (
          <p className="truncate font-mono text-meta text-fg-muted">{[...sel.keys()][0] ?? t("picker.noneSelected")}</p>
        )}
      </div>
    );

  return (
    <RDialog.Root open={open} onOpenChange={onOpenChange}>
      <RDialog.Portal>
        <RDialog.Overlay className="fixed inset-0 z-(--z-overlay) bg-overlay" />
        <RDialog.Content
          aria-describedby={undefined}
          className={cn(
            "fixed z-(--z-overlay) flex flex-col border border-border bg-surface-raised shadow-overlay",
            narrow ? "inset-0" : "top-1/2 left-1/2 h-[70dvh] w-[calc(100vw-2rem)] max-w-3xl -translate-x-1/2 -translate-y-1/2 rounded-lg",
          )}
        >
          <div className="flex items-center gap-3 border-b border-border px-4 py-3">
            <RDialog.Title className="min-w-0 flex-1 truncate text-title font-semibold">{title ?? t(`picker.title.${mode}${gitOnly ? "Git" : ""}`)}</RDialog.Title>
            <RDialog.Close asChild>
              <IconButton size="sm" label={t("picker.close")} icon={<X className="size-4" />} />
            </RDialog.Close>
          </div>
          {body}
          {tray}
          {mode !== "browse" ? (
            <div className="flex justify-end gap-2 border-t border-border px-4 py-3">
              <Button variant="ghost" onClick={() => onOpenChange(false)}>
                {t("picker.cancel")}
              </Button>
              <Button
                variant="primary"
                disabled={sel.size === 0}
                onClick={() => {
                  onConfirm?.(sel);
                  onOpenChange(false);
                }}
              >
                {t("picker.choose")}
              </Button>
            </div>
          ) : null}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}

