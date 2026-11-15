// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { ApiError } from "@/shared/api";
import { BrandMark, Button, Input } from "@/shared/ui";
import "../texts";
import { useOperatorLogin } from "../api";
import { normaliseCode } from "../model";

/** The operator's login: password and a TOTP or backup code, in one form. */
export default function OperatorLoginPage() {
  const { t } = useTranslation("operator");
  const navigate = useNavigate();
  const login = useOperatorLogin();
  const [handle, setHandle] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    login.mutate(
      { handle: handle.trim().toLowerCase(), password, code: normaliseCode(code) },
      { onSuccess: () => navigate("/", { replace: true }), onError: () => setCode("") },
    );
  };
  const err = login.error;
  const error = err
    ? err instanceof ApiError && err.status === 401
      ? t("login.failed")
      : err instanceof ApiError && err.status === 429
        ? t("login.locked")
        : t("login.error")
    : null;

  return (
    <main className="grid min-h-full place-items-center bg-bg px-4">
      <form onSubmit={submit} className="flex w-full max-w-sm flex-col gap-4 rounded-lg border border-border bg-surface p-6 shadow-raised">
        <div className="flex items-center gap-2">
          <BrandMark className="size-7" />
          <span className="text-title font-semibold">{t("title")}</span>
        </div>
        <h1 className="text-page font-semibold">{t("login.title")}</h1>
        <label className="flex flex-col gap-1 text-ui">
          {t("login.handle")}
          <Input autoComplete="username" autoCapitalize="none" required value={handle} onChange={(e) => setHandle(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1 text-ui">
          {t("login.password")}
          <Input type="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1 text-ui">
          {t("login.code")}
          <Input autoComplete="one-time-code" inputMode="text" autoCapitalize="characters" spellCheck={false} required value={code} onChange={(e) => setCode(e.target.value)} />
          <span className="text-meta text-fg-subtle">{t("login.codeHelp")}</span>
        </label>
        {error ? (
          <p role="alert" className="text-ui text-danger">
            {error}
          </p>
        ) : null}
        <Button type="submit" variant="primary" loading={login.isPending}>
          {t("login.submit")}
        </Button>
      </form>
    </main>
  );
}
