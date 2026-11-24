// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { ApiError } from "@/shared/api";
import { fullTime } from "@/shared/lib";
import { BrandMark, Button, Input, Skeleton } from "@/shared/ui";
import { useAcceptInvite, useInviteLookup } from "../api";
import { inviteToken, MIN_OWNER_PASSWORD, validOwnerHandle } from "../model";

/**
 * The owner's invite (round 51): /app/invite#<token>, from the operator.
 * The token stays in the fragment, so it is never in a server log.
 */
export default function InvitePage() {
  const { t, i18n } = useTranslation("identity");
  const navigate = useNavigate();
  const [token] = useState(() => inviteToken(globalThis.location?.hash ?? ""));
  const lookup = useInviteLookup(token);
  const accept = useAcceptInvite();
  const [handle, setHandle] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");

  const cleanHandle = handle.trim().replace(/^@/, "").toLowerCase();
  const handleBad = cleanHandle !== "" && !validOwnerHandle(cleanHandle);
  const short = password !== "" && password.length < MIN_OWNER_PASSWORD;
  const mismatch = again !== "" && again !== password;
  const ready = validOwnerHandle(cleanHandle) && password.length >= MIN_OWNER_PASSWORD && password === again;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) return;
    accept.mutate(
      { token, handle: cleanHandle, display_name: name.trim() || undefined, password },
      {
        onSuccess: () => {
          // The token must not stay in the address bar or the history.
          globalThis.history?.replaceState(null, "", "/agent/");
          navigate("/", { replace: true });
        },
      },
    );
  };

  const gone = (e: unknown) => e instanceof ApiError && e.status === 410;
  const err = accept.error;
  const error = err ? (gone(err) ? t("invite.gone") : err instanceof ApiError && err.status === 409 ? t("invite.handleTaken") : err.message) : null;

  return (
    <main className="grid min-h-full place-items-center px-4">
      <div className="flex w-full max-w-sm flex-col gap-4 rounded-lg border border-border bg-surface p-6 shadow-raised">
        <div className="flex items-center gap-2">
          <BrandMark className="size-7" />
          <span className="text-title font-semibold">{t("common:appName")}</span>
        </div>
        {!token || lookup.isError ? (
          <>
            <h1 className="text-page font-semibold">{t("invite.invalidTitle")}</h1>
            <p role="alert" className="text-ui text-fg-muted">
              {!token || gone(lookup.error) ? t("invite.gone") : t("invite.error")}
            </p>
          </>
        ) : lookup.isPending ? (
          <Skeleton className="h-40" />
        ) : (
          <form onSubmit={submit} className="flex flex-col gap-4">
            <div>
              <h1 className="text-page font-semibold">{t("invite.title", { workspace: lookup.data.workspace_name })}</h1>
              <p className="text-meta text-fg-muted">{t("invite.body", { expires: fullTime(lookup.data.expires_at, i18n.language) })}</p>
            </div>
            <label className="flex flex-col gap-1 text-ui">
              {t("invite.handle")}
              <Input autoComplete="username" autoCapitalize="none" spellCheck={false} required value={handle} aria-invalid={handleBad} onChange={(e) => setHandle(e.target.value)} />
              <span className={handleBad ? "text-meta text-danger" : "text-meta text-fg-subtle"}>{t("invite.handleHelp")}</span>
            </label>
            <label className="flex flex-col gap-1 text-ui">
              {t("invite.name")}
              <Input autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            <label className="flex flex-col gap-1 text-ui">
              {t("invite.password")}
              <Input type="password" autoComplete="new-password" required value={password} aria-invalid={short} onChange={(e) => setPassword(e.target.value)} />
              <span className={short ? "text-meta text-danger" : "text-meta text-fg-subtle"}>{t("invite.passwordHelp", { min: MIN_OWNER_PASSWORD })}</span>
            </label>
            <label className="flex flex-col gap-1 text-ui">
              {t("invite.again")}
              <Input type="password" autoComplete="new-password" required value={again} aria-invalid={mismatch} onChange={(e) => setAgain(e.target.value)} />
              {mismatch ? <span className="text-meta text-danger">{t("invite.mismatch")}</span> : null}
            </label>
            {error ? (
              <p role="alert" className="text-ui text-danger">
                {error}
              </p>
            ) : null}
            <Button type="submit" variant="primary" loading={accept.isPending} disabled={!ready}>
              {t("invite.submit")}
            </Button>
          </form>
        )}
      </div>
    </main>
  );
}
