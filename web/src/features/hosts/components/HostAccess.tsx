// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { KeyRound } from "lucide-react";
import { useActors, useSession } from "@/features/identity";
import { Badge, Button, Dialog, Input, Notice, Select, toast } from "@/shared/ui";
import { loginFailure, useClearClaudeLogin, useSetClaudeLogin, useSetHostOwner, type Host, type LoginFailure } from "../api";
import { stamp } from "./bits";
import { Row } from "./OverviewTab";

/** Tulajdonos: who the machine belongs to; the workspace owner changes it (round 38). */
export function OwnerRow({ host }: { host: Host }) {
  const { t } = useTranslation("hosts");
  const session = useSession();
  const actors = useActors();
  const setOwner = useSetHostOwner();
  const isOwner = session.data?.actor.role === "owner";
  const people = [...(actors.data?.values() ?? [])].filter((a) => a.kind === "human" && !a.archived_at);
  const current = host.owner_id ? actors.data?.get(host.owner_id) : undefined;

  if (!isOwner) {
    return <Row label={t("access.owner")}>{current ? `@${current.handle}` : t("access.nobody")}</Row>;
  }
  return (
    <Row label={t("access.owner")}>
      <Select
        aria-label={t("access.owner")}
        value={host.owner_id ?? ""}
        disabled={setOwner.isPending}
        onChange={(e) =>
          setOwner.mutate(
            { hostId: host.id, ownerId: e.target.value || null },
            { onSuccess: () => toast(t("access.ownerSaved")), onError: () => toast(t("access.ownerFailed"), { tone: "danger" }) },
          )
        }
      >
        <option value="">{t("access.nobody")}</option>
        {people.map((p) => (
          <option key={p.id} value={p.id}>
            {p.display_name} (@{p.handle})
          </option>
        ))}
      </Select>
    </Row>
  );
}

/** Whether the viewer may set this machine's Claude login: its owner or an admin. */
export function mayManageLogin(host: Host, me: { id: string; role: string } | undefined): boolean {
  if (!me) return false;
  return me.role === "owner" || me.role === "admin" || (Boolean(host.owner_id) && host.owner_id === me.id);
}

/** Claude-belépés: whether the guard has a login token, and the Dialog that sets it. */
export function ClaudeLoginRow({ host }: { host: Host }) {
  const { t, i18n } = useTranslation("hosts");
  const session = useSession();
  const me = session.data?.actor;
  const [open, setOpen] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const clear = useClearClaudeLogin();
  const r = host.report;
  if (!mayManageLogin(host, me)) return null;

  const state = !r.claude_login_configured ? (
    <span className="text-fg-muted">{t("access.notConfigured")}</span>
  ) : r.claude_login_set_at ? (
    <Badge tone="success">{t("access.set", { when: stamp(r.claude_login_set_at, i18n.language) })}</Badge>
  ) : (
    <Badge tone="warning">{t("access.unset")}</Badge>
  );

  return (
    <Row label={t("access.login")}>
      <div className="flex flex-col gap-2">
        <span>{state}</span>
        {r.claude_login_configured ? (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" icon={<KeyRound className="size-4" />} onClick={() => setOpen(true)}>
              {t("access.setButton")}
            </Button>
            {r.claude_login_set_at ? (
              <Button size="sm" variant="danger" onClick={() => setConfirmClear(true)}>
                {t("access.clearButton")}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      <ClaudeLoginDialog host={host} open={open} onOpenChange={setOpen} />
      <Dialog
        open={confirmClear}
        onOpenChange={setConfirmClear}
        title={t("access.clearTitle", { name: host.name })}
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirmClear(false)}>{t("cancel")}</Button>
            <Button
              variant="danger"
              loading={clear.isPending}
              onClick={() =>
                clear.mutate(host.id, {
                  onSuccess: () => toast(t("access.cleared")),
                  onError: (err) => toast(t(`access.errors.${loginFailure(err)}`), { tone: "danger" }),
                  onSettled: () => setConfirmClear(false),
                })
              }
            >
              {t("access.clearConfirm")}
            </Button>
          </>
        }
      >
        <p className="text-ui">{t("access.clearBody")}</p>
      </Dialog>
    </Row>
  );
}

/**
 * Belépés beállítása: the token from `claude setup-token`, typed into a
 * password field. It goes only into the request; the field is emptied on
 * submit and the mutation is reset, so nothing keeps it.
 */
export function ClaudeLoginDialog({ host, open, onOpenChange }: { host: Host; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation("hosts");
  const set = useSetClaudeLogin();
  const [token, setToken] = useState("");
  const [failure, setFailure] = useState<LoginFailure | null>(null);

  const close = (next: boolean) => {
    if (!next) {
      setToken("");
      setFailure(null);
      set.reset();
    }
    onOpenChange(next);
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const value = token.trim();
    setToken("");
    setFailure(null);
    if (!value) return;
    set.mutate(
      { hostId: host.id, token: value },
      {
        onSuccess: () => {
          toast(t("access.saved"));
          close(false);
        },
        onError: (err) => setFailure(loginFailure(err)),
        onSettled: () => set.reset(),
      },
    );
  };

  return (
    <Dialog
      open={open}
      onOpenChange={close}
      title={t("access.dialogTitle", { name: host.name })}
      description={t("access.dialogBody")}
      closeLabel={t("close")}
      footer={
        <>
          <Button onClick={() => close(false)}>{t("cancel")}</Button>
          <Button variant="primary" type="submit" form={`claude-login-${host.id}`} loading={set.isPending} disabled={!token.trim() || set.isPending}>
            {t("access.save")}
          </Button>
        </>
      }
    >
      <form id={`claude-login-${host.id}`} onSubmit={submit} className="flex flex-col gap-3">
        <ol className="list-decimal pl-5 text-ui">
          <li>{t("access.step1")}</li>
          <li>
            <code className="font-mono">claude setup-token</code> – {t("access.step2")}
          </li>
          <li>{t("access.step3")}</li>
        </ol>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("access.tokenLabel")}</span>
          <Input
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={token}
            disabled={set.isPending}
            onChange={(e) => setToken(e.target.value)}
          />
        </label>
        {set.isPending ? <p className="text-ui text-fg-muted">{t("access.checking")}</p> : null}
        {failure ? (
          <Notice tone="danger">
            {t(`access.errors.${failure}`)}
          </Notice>
        ) : null}
        <p className="text-meta text-fg-subtle">{t("access.privacy")}</p>
      </form>
    </Dialog>
  );
}
