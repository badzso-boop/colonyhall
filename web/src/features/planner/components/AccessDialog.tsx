import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Search, ShieldCheck } from "lucide-react";
import { cn, hueVar } from "@/shared/lib";
import { Avatar, Button, Checkbox, Dialog, Input, Notice, Skeleton, toast } from "@/shared/ui";
import { useActors, useSession } from "@/features/identity";
import { usePlannerAccess, useSavePlannerAccess } from "../api";
import { accessCandidates, accessDraft, applyAccess, toggleId, type AccessDraft, type AccessKind } from "../access";

/** The agent multi-picker: a filter and a checkbox per agent. */
function AgentPicker({ ids, onToggle }: { ids: string[]; onToggle: (id: string) => void }) {
  const { t } = useTranslation("planner");
  const actors = useActors();
  const [query, setQuery] = useState("");
  const list = accessCandidates(actors.data?.values() ?? [], ids, query);
  return (
    <div className="flex flex-col gap-2">
      <label className="relative">
        <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
        <Input type="search" aria-label={t("access.search")} placeholder={t("access.search")} value={query} onChange={(e) => setQuery(e.target.value)} className="pl-8" />
      </label>
      <ul aria-label={t("access.agents")} className="flex max-h-72 flex-col gap-px overflow-y-auto">
        {list.map((a) => (
          <li key={a.id}>
            <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 hover:bg-surface-sunken">
              <Checkbox checked={ids.includes(a.id)} onCheckedChange={() => onToggle(a.id)} aria-label={`@${a.handle}`} />
              <Avatar kind="agent" name={a.handle} emoji={a.avatar_emoji} size="sm" ringColor={hueVar(a.effective_color)} />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{a.display_name || a.handle}</span>
                <span className="block truncate font-mono text-meta text-fg-muted">@{a.handle}</span>
              </span>
            </label>
          </li>
        ))}
        {actors.data && list.length === 0 ? <li className="px-2 py-3 text-meta text-fg-subtle">{t("access.none")}</li> : null}
      </ul>
      <p className="text-meta text-fg-muted">{t("access.chosen", { count: ids.length })}</p>
    </div>
  );
}

function AccessDialog({ kind, onClose }: { kind: AccessKind; onClose: () => void }) {
  const { t } = useTranslation("planner");
  const access = usePlannerAccess(true);
  const save = useSavePlannerAccess();
  // Edited copy; until the first edit it follows what the server holds.
  const [edited, setEdited] = useState<AccessDraft | null>(null);
  const draft = edited ?? (access.data ? accessDraft(access.data, kind) : null);
  const modes = kind === "tasks" ? (["all", "selected"] as const) : (["none", "selected"] as const);
  const submit = () => {
    if (!access.data || !draft) return;
    save.mutate(applyAccess(access.data, kind, draft), {
      onSuccess: () => {
        toast(t("access.saved"));
        onClose();
      },
      onError: () => toast(t("access.failed"), { tone: "danger" }),
    });
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t(`access.title.${kind}`)}
      description={t(`access.body.${kind}`)}
      closeLabel={t("access.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("access.cancel")}
          </Button>
          <Button variant="primary" disabled={!draft} loading={save.isPending} onClick={submit}>
            {t("access.save")}
          </Button>
        </>
      }
    >
      {access.isError ? <Notice tone="danger">{t("access.loadFailed")}</Notice> : null}
      {!draft && !access.isError ? <Skeleton className="h-24 w-full" /> : null}
      {draft ? (
        <div className="flex flex-col gap-3">
          <div role="radiogroup" aria-label={t(`access.title.${kind}`)} className="flex flex-col gap-1.5">
            {modes.map((m) => (
              <button
                key={m}
                type="button"
                role="radio"
                aria-checked={draft.mode === m}
                onClick={() => setEdited({ ...draft, mode: m })}
                className={cn("rounded-md border px-3 py-2 text-left text-ui", draft.mode === m ? "border-accent bg-accent-soft font-semibold" : "border-border")}
              >
                {t(`access.mode.${kind}.${m}`)}
              </button>
            ))}
          </div>
          {draft.mode === "selected" ? <AgentPicker ids={draft.ids} onToggle={(id) => setEdited({ ...draft, ids: toggleId(draft.ids, id) })} /> : null}
        </div>
      ) : null}
    </Dialog>
  );
}

/**
 * "Hozzáférés" on the Feladatok and Naptár pages (docs/handoff/34, part 8):
 * which agents may read the owner's tasks or calendar. Every human owns
 * their own planner; agents do not get the button.
 */
export function AccessButton({ kind }: { kind: AccessKind }) {
  const { t } = useTranslation("planner");
  const actor = useSession().data?.actor;
  const [open, setOpen] = useState(false);
  if (actor?.kind !== "human") return null;
  return (
    <>
      <Button size="sm" icon={<ShieldCheck aria-hidden className="size-3.5" />} onClick={() => setOpen(true)}>
        {t("access.button")}
      </Button>
      {open ? <AccessDialog kind={kind} onClose={() => setOpen(false)} /> : null}
    </>
  );
}
