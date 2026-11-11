import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, KeyRound, Plus } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import type { Schemas } from "@/shared/api";
import { useLocalStorage } from "@/shared/hooks";
import { cn, fullTime } from "@/shared/lib";
import { Badge, Button, Checkbox, Dialog, EmptyState, Input, Notice, Select, Skeleton, toast } from "@/shared/ui";
import { useIssueToken, useRevokeToken, useTokens } from "../api";
import { AdminPage } from "../components/AdminPage";
import { SecretOnce } from "../components/SecretOnce";
import { budapestDay, wizardPlaces, wizardReducer, wizardStart, type WizardAction, type WizardState } from "../model";

type Token = Schemas["Token"];

/** Tokenek (/admin/tokens): your own, others' as far as the server allows, and the owner key rotation. */
export default function TokensPage() {
  const { t } = useTranslation("admin");
  return (
    <AdminPage icon={<KeyRound className="size-5" />} title={t("tokens.title")}>
      <TokensBody />
    </AdminPage>
  );
}

function TokensBody() {
  const { t } = useTranslation("admin");
  const session = useSession();
  const me = session.data?.actor;
  const owner = me?.role === "owner";
  const [issuing, setIssuing] = useState(false);
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4">
      <div className="mx-auto flex max-w-3xl flex-col gap-6">
        {owner ? <OwnerKeyWizard actorId={me?.id} /> : null}
        <section className="flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <h2 className="mr-auto font-semibold">{t("tokens.mine")}</h2>
            <Button size="sm" variant="primary" icon={<Plus className="size-4" />} onClick={() => setIssuing(true)}>
              {t("tokens.new")}
            </Button>
          </div>
          <TokenList actorId={me?.id} />
        </section>
        <OthersTokens meId={me?.id} />
      </div>
      {issuing ? <IssueDialog actorId={me?.id} onClose={() => setIssuing(false)} /> : null}
    </div>
  );
}

function TokenList({ actorId, hide }: { actorId: string | undefined; hide?: string | null }) {
  const { t, i18n } = useTranslation("admin");
  const tokens = useTokens(actorId);
  const [revoking, setRevoking] = useState<Token | null>(null);
  if (tokens.isPending) return <Skeleton className="h-16 w-full" />;
  if (tokens.isError) return <p className="text-ui text-danger">{tokens.error.message}</p>;
  const list = tokens.data.filter((x) => x.id !== hide);
  if (!list.length) return <EmptyState title={t("tokens.empty")} />;
  return (
    <>
      <ul className="flex flex-col divide-y divide-border rounded-lg border border-border bg-surface">
        {list.map((tok) => (
          <li key={tok.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-ui">
            <span className="min-w-0 flex-1 basis-40 truncate font-medium">
              {tok.label || t("tokens.noLabel")}
              {tok.prefix ? <code className="ml-2 font-mono text-meta text-fg-subtle">{tok.prefix}…</code> : null}
            </span>
            <span className="flex flex-col text-right text-meta text-fg-muted">
              <span>{t("tokens.created", { when: fullTime(tok.created_at, i18n.language) })}</span>
              <span>{tok.last_used_at ? t("tokens.lastUsed", { when: fullTime(tok.last_used_at, i18n.language) }) : t("tokens.neverUsed")}</span>
            </span>
            {tok.revoked_at ? (
              <Badge tone="neutral">{t("tokens.revoked")}</Badge>
            ) : (
              <Button size="sm" variant="ghost" onClick={() => setRevoking(tok)}>
                {t("tokens.revoke")}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {revoking ? <RevokeDialog token={revoking} onClose={() => setRevoking(null)} /> : null}
    </>
  );
}

function RevokeDialog({ token, onClose, onDone }: { token: Token; onClose: () => void; onDone?: () => void }) {
  const { t } = useTranslation("admin");
  const revoke = useRevokeToken();
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("tokens.revokeTitle", { name: token.label || t("tokens.noLabel") })}
      closeLabel={t("close")}
      footer={
        <Button
          variant="danger"
          loading={revoke.isPending}
          onClick={() =>
            revoke.mutate(token.id, {
              onSuccess: () => {
                onDone?.();
                onClose();
              },
              onError: (e) => toast(e.message, { tone: "danger" }),
            })
          }
        >
          {t("tokens.revoke")}
        </Button>
      }
    >
      <p>{t("tokens.revokeBody")}</p>
    </Dialog>
  );
}

function IssueDialog({ actorId, onClose }: { actorId: string | undefined; onClose: () => void }) {
  const { t } = useTranslation("admin");
  const issue = useIssueToken(actorId);
  const [label, setLabel] = useState("");
  const [secret, setSecret] = useState<string | null>(null);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("tokens.newTitle")} closeLabel={t("close")}>
      {secret ? (
        <div className="flex flex-col gap-3">
          <SecretOnce secret={secret} />
          <div className="flex justify-end">
            <Button variant="primary" onClick={onClose}>
              {t("done")}
            </Button>
          </div>
        </div>
      ) : (
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            issue.mutate(label.trim(), { onSuccess: (r) => setSecret(r.secret) });
          }}
        >
          {issue.error ? <Notice tone="danger">{issue.error.message}</Notice> : null}
          <label className="flex flex-col gap-1 text-ui">
            <span className="font-medium">{t("tokens.label")}</span>
            <Input value={label} autoFocus maxLength={64} onChange={(e) => setLabel(e.target.value)} />
          </label>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              {t("cancel")}
            </Button>
            <Button type="submit" variant="primary" disabled={!label.trim()} loading={issue.isPending}>
              {t("tokens.issue")}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function OthersTokens({ meId }: { meId: string | undefined }) {
  const { t } = useTranslation("admin");
  const actors = useActors();
  const [actorId, setActorId] = useState("");
  const others = [...(actors.data?.values() ?? [])].filter((a) => a.id !== meId && !a.archived_at);
  return (
    <section className="flex flex-col gap-2">
      <h2 className="font-semibold">{t("tokens.others")}</h2>
      <Select aria-label={t("tokens.pickActor")} value={actorId} onChange={(e) => setActorId(e.target.value)} className="w-64 max-w-full">
        <option value="">{t("tokens.pickActor")}</option>
        {others.map((a) => (
          <option key={a.id} value={a.id}>
            @{a.handle}
          </option>
        ))}
      </Select>
      {actorId ? <TokenList actorId={actorId} /> : null}
    </section>
  );
}

const WIZARD_KEY = "colony.ownerKeyWizard";

/** The guided rotation (round 35, part 5). Only ids and ticks are remembered, never the secret. */
function OwnerKeyWizard({ actorId }: { actorId: string | undefined }) {
  const { t } = useTranslation("admin");
  const [state, setState] = useLocalStorage<WizardState>(WIZARD_KEY, wizardStart);
  const dispatch = (a: WizardAction) => setState(wizardReducer(state, a));
  const [open, setOpen] = useState(state.step > 1 || state.newTokenId !== null);
  if (!open) {
    return (
      <section className="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-surface p-3">
        <div className="min-w-0 flex-1 basis-64">
          <h2 className="font-semibold">{t("wizard.title")}</h2>
          <p className="text-ui text-fg-muted">{t(state.done ? "wizard.doneBody" : "wizard.intro")}</p>
        </div>
        <Button
          onClick={() => {
            if (state.done) dispatch({ type: "restart" });
            setOpen(true);
          }}
        >
          {t("wizard.start")}
        </Button>
      </section>
    );
  }
  const steps = [1, 2, 3] as const;
  return (
    <section aria-label={t("wizard.title")} className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-3">
      <h2 className="font-semibold">{t("wizard.title")}</h2>
      <ol className="flex flex-wrap gap-x-4 gap-y-1 text-meta">
        {steps.map((n) => (
          <li key={n} aria-current={state.step === n ? "step" : undefined} className={cn("flex items-center gap-1.5", state.step === n ? "font-semibold text-fg" : "text-fg-subtle")}>
            <span className={cn("grid size-5 place-items-center rounded-full border", state.step === n ? "border-accent" : "border-border")}>{state.step > n ? <Check aria-hidden className="size-3" /> : n}</span>
            {t(`wizard.step${n}`)}
          </li>
        ))}
      </ol>
      {state.step === 1 ? <StepCreate actorId={actorId} state={state} dispatch={dispatch} /> : null}
      {state.step === 2 ? <StepPlaces state={state} dispatch={dispatch} /> : null}
      {state.step === 3 ? (
        <StepRevoke
          actorId={actorId}
          state={state}
          dispatch={dispatch}
          onFinish={() => {
            dispatch({ type: "finish" });
            setOpen(false);
          }}
        />
      ) : null}
    </section>
  );
}

function StepCreate({ actorId, state, dispatch }: { actorId: string | undefined; state: WizardState; dispatch: (a: WizardAction) => void }) {
  const { t } = useTranslation("admin");
  const issue = useIssueToken(actorId);
  const revoke = useRevokeToken();
  const [secret, setSecret] = useState<string | null>(null);
  const create = async () => {
    // A key from an abandoned try was never saved, and its secret is gone.
    if (state.newTokenId && !state.saved) await revoke.mutateAsync(state.newTokenId).catch(() => undefined);
    const label = `owner-${budapestDay(new Date())}`;
    const r = await issue.mutateAsync(label);
    setSecret(r.secret);
    dispatch({ type: "created", tokenId: r.token.id });
  };
  const lost = !secret && state.newTokenId !== null && !state.saved;
  return (
    <div className="flex flex-col gap-3 text-ui">
      {lost ? <Notice tone="warning">{t("wizard.lost")}</Notice> : <p>{t("wizard.createBody")}</p>}
      {issue.error ? <Notice tone="danger">{issue.error.message}</Notice> : null}
      {secret ? <SecretOnce secret={secret} /> : null}
      {!secret && !state.saved ? (
        <div>
          <Button variant="primary" loading={issue.isPending || revoke.isPending} onClick={() => void create().catch(() => undefined)}>
            {t("wizard.create")}
          </Button>
        </div>
      ) : null}
      {secret || state.saved ? (
        <label className="flex items-center gap-2">
          <Checkbox checked={state.saved} onCheckedChange={(v) => dispatch({ type: "saved", saved: v === true })} />
          {t("wizard.saved")}
        </label>
      ) : null}
      <div>
        <Button variant="primary" disabled={!state.saved} onClick={() => dispatch({ type: "goto", step: 2 })}>
          {t("wizard.next")}
        </Button>
      </div>
    </div>
  );
}

function StepPlaces({ state, dispatch }: { state: WizardState; dispatch: (a: WizardAction) => void }) {
  const { t } = useTranslation("admin");
  return (
    <div className="flex flex-col gap-3 text-ui">
      <p>{t("wizard.placesBody")}</p>
      <ul className="flex flex-col gap-1.5">
        {wizardPlaces.map((p) => (
          <li key={p}>
            <label className="flex items-center gap-2">
              <Checkbox checked={state.places.includes(p)} onCheckedChange={(v) => dispatch({ type: "place", place: p, on: v === true })} />
              {t(`wizard.places.${p}`)}
            </label>
          </li>
        ))}
      </ul>
      <Notice>{t("wizard.exchange")}</Notice>
      <div className="flex gap-2">
        <Button variant="ghost" onClick={() => dispatch({ type: "goto", step: 1 })}>
          {t("wizard.back")}
        </Button>
        <Button variant="primary" onClick={() => dispatch({ type: "goto", step: 3 })}>
          {t("wizard.next")}
        </Button>
      </div>
    </div>
  );
}

function StepRevoke({ actorId, state, dispatch, onFinish }: { actorId: string | undefined; state: WizardState; dispatch: (a: WizardAction) => void; onFinish: () => void }) {
  const { t, i18n } = useTranslation("admin");
  const tokens = useTokens(actorId);
  const [revoking, setRevoking] = useState<Token | null>(null);
  const old = (tokens.data ?? []).filter((x) => !x.revoked_at && x.id !== state.newTokenId);
  return (
    <div className="flex flex-col gap-3 text-ui">
      <p>{t("wizard.revokeBody")}</p>
      {old.length ? (
        <ul className="flex flex-col divide-y divide-border">
          {old.map((tok) => (
            <li key={tok.id} className="flex flex-wrap items-center gap-x-3 py-2">
              <span className="min-w-0 flex-1 basis-40 truncate font-medium">
                {tok.label || t("tokens.noLabel")}
                {tok.prefix ? <code className="ml-2 font-mono text-meta text-fg-subtle">{tok.prefix}…</code> : null}
              </span>
              <span className="text-meta text-fg-muted">
                {tok.last_used_at ? t("tokens.lastUsed", { when: fullTime(tok.last_used_at, i18n.language) }) : t("tokens.created", { when: fullTime(tok.created_at, i18n.language) })}
              </span>
              <Button size="sm" variant="danger" onClick={() => setRevoking(tok)}>
                {t("wizard.revokeOld")}
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <Notice tone="success">{t("wizard.noneLeft")}</Notice>
      )}
      <div className="flex gap-2">
        <Button variant="ghost" onClick={() => dispatch({ type: "goto", step: 2 })}>
          {t("wizard.back")}
        </Button>
        <Button variant="primary" onClick={onFinish}>
          {t("wizard.finish")}
        </Button>
      </div>
      {revoking ? <RevokeDialog token={revoking} onClose={() => setRevoking(null)} onDone={() => toast(t("wizard.revoked"))} /> : null}
    </div>
  );
}
