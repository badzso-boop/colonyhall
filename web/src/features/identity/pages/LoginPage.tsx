// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router";
import { ApiError } from "@/shared/api";
import { BrandMark, Button, Input } from "@/shared/ui";
import { useLogin } from "../api";
import { SourceLink } from "../components/SourceLink";
import { safeNext } from "../model";

export default function LoginPage() {
  const { t } = useTranslation("identity");
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const login = useLogin();
  const [handle, setHandle] = useState("");
  const [password, setPassword] = useState("");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    login.mutate(
      { handle: handle.trim().replace(/^@/, "").toLowerCase(), password },
      { onSuccess: () => navigate(safeNext(params.get("next")), { replace: true }) },
    );
  };

  const error = login.error
    ? login.error instanceof ApiError && login.error.status === 401
      ? t("login.failed")
      : t("login.error")
    : null;

  return (
    <main className="grid min-h-full place-items-center px-4">
      <div className="flex w-full max-w-sm flex-col items-center gap-3">
        <form onSubmit={submit} className="flex w-full max-w-sm flex-col gap-4 rounded-lg border border-border bg-surface p-6 shadow-raised">
          <div className="flex items-center gap-2">
            <BrandMark className="size-7" />
            <span className="text-title font-semibold">{t("common:appName")}</span>
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
          {error ? (
            <p role="alert" className="text-ui text-danger">
              {error}
            </p>
          ) : null}
          <Button type="submit" variant="primary" loading={login.isPending}>
            {t("login.submit")}
          </Button>
        </form>
        <footer>
          <SourceLink />
        </footer>
      </div>
    </main>
  );
}
