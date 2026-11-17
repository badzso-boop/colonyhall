// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { Schemas } from "@/shared/api";
import { Button, Dialog, Input, Notice, Select } from "@/shared/ui";
import { useCreateConnector, useUpdateConnector, type Connector } from "../api";
import { slugFrom, validSlug, validURL } from "../model";

type Auth = Schemas["Connector"]["auth"];

/** Creates or edits a connector; the secret is write-only. */
export function ConnectorDialog({
  connector,
  secretsAvailable,
  onClose,
}: {
  connector: Connector | null;
  secretsAvailable: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation("workshop");
  const editing = connector !== null;
  const create = useCreateConnector();
  const update = useUpdateConnector(connector?.id ?? "");
  const [title, setTitle] = useState(connector?.title ?? "");
  const [slug, setSlug] = useState(connector?.slug ?? "");
  const [slugTouched, setSlugTouched] = useState(editing);
  const [kind, setKind] = useState<"mcp" | "http">(connector?.kind ?? "mcp");
  const [url, setUrl] = useState(connector?.base_url ?? "");
  const [auth, setAuth] = useState<Auth>(connector?.auth ?? "none");
  const [header, setHeader] = useState(connector?.header_name ?? "");
  const [secret, setSecret] = useState("");
  const [tried, setTried] = useState(false);
  const shownSlug = slugTouched ? slug : slugFrom(title);
  const pending = create.isPending || update.isPending;
  const error = create.error ?? update.error;
  const slugOK = editing || validSlug(shownSlug);
  const urlOK = validURL(url);
  const titleOK = title.trim().length > 0 && title.length <= 80;
  const needsSecret =
    auth !== "none" &&
    !(editing && connector?.has_secret && connector.auth === auth);
  const secretOK = !needsSecret || secret.length > 0;

  const submit = () => {
    setTried(true);
    if (!slugOK || !urlOK || !titleOK || !secretOK) return;
    const common = {
      title: title.trim(),
      base_url: url.trim(),
      auth,
      ...(auth === "header" ? { header_name: header.trim() } : {}),
      ...(secret ? { secret } : {}),
    };
    if (editing) update.mutate(common, { onSuccess: onClose });
    else
      create.mutate(
        { slug: shownSlug, kind, ...common },
        { onSuccess: onClose },
      );
  };

  return (
    <Dialog
      open
      size="lg"
      onOpenChange={(o) => !o && onClose()}
      title={
        editing
          ? t("dialog.editTitle", { name: connector.title })
          : t("dialog.newTitle")
      }
      closeLabel={t("close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("dialog.cancel")}
          </Button>
          <Button variant="primary" loading={pending} onClick={submit}>
            {editing ? t("dialog.save") : t("dialog.create")}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          submit();
        }}
      >
        {error ? <Notice tone="danger">{error.message}</Notice> : null}
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("dialog.name")}</span>
          <Input
            value={title}
            autoFocus
            maxLength={80}
            onChange={(e) => setTitle(e.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("dialog.slug")}</span>
          <Input
            value={shownSlug}
            disabled={editing}
            maxLength={16}
            className="font-mono"
            onChange={(e) => {
              setSlugTouched(true);
              setSlug(e.target.value);
            }}
          />
          <span
            className={
              tried && !slugOK
                ? "text-meta text-danger"
                : "text-meta text-fg-muted"
            }
          >
            {tried && !slugOK
              ? t("dialog.slugBad")
              : t("dialog.slugHint", { slug: shownSlug || "…" })}
          </span>
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("dialog.kind")}</span>
          <Select
            value={kind}
            disabled={editing}
            onChange={(e) => setKind(e.target.value as "mcp" | "http")}
          >
            <option value="mcp">{t("connectors.kind.mcp")}</option>
            <option value="http">{t("connectors.kind.http")}</option>
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("dialog.url")}</span>
          <Input
            value={url}
            inputMode="url"
            placeholder="https://mcp.example.com/mcp"
            className="font-mono"
            onChange={(e) => setUrl(e.target.value)}
          />
          <span
            className={
              tried && !urlOK
                ? "text-meta text-danger"
                : "text-meta text-fg-muted"
            }
          >
            {tried && !urlOK ? t("dialog.urlBad") : t("dialog.urlHint")}
          </span>
        </label>
        <label className="flex flex-col gap-1 text-ui">
          <span className="font-medium">{t("dialog.auth")}</span>
          <Select
            value={auth}
            onChange={(e) => setAuth(e.target.value as Auth)}
          >
            <option value="none">{t("dialog.authNone")}</option>
            <option value="bearer" disabled={!secretsAvailable}>
              {t("dialog.authBearer")}
            </option>
            <option value="header" disabled={!secretsAvailable}>
              {t("dialog.authHeader")}
            </option>
            <option value="basic" disabled={!secretsAvailable}>
              {t("dialog.authBasic")}
            </option>
          </Select>
        </label>
        {auth === "header" ? (
          <label className="flex flex-col gap-1 text-ui">
            <span className="font-medium">{t("dialog.header")}</span>
            <Input
              value={header}
              maxLength={64}
              placeholder="X-Api-Key"
              className="font-mono"
              onChange={(e) => setHeader(e.target.value)}
            />
          </label>
        ) : null}
        {auth !== "none" ? (
          <label className="flex flex-col gap-1 text-ui">
            <span className="font-medium">{t("dialog.secret")}</span>
            <Input
              type="password"
              autoComplete="new-password"
              value={secret}
              maxLength={8192}
              placeholder={
                editing && connector?.has_secret && connector.auth === auth
                  ? t("dialog.secretKeep", {
                      hint: connector.secret_hint ?? "",
                    })
                  : auth === "basic"
                    ? t("dialog.secretBasic")
                    : ""
              }
              onChange={(e) => setSecret(e.target.value)}
            />
            <span
              className={
                tried && !secretOK
                  ? "text-meta text-danger"
                  : "text-meta text-fg-muted"
              }
            >
              {t("dialog.secretHint")}
            </span>
          </label>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
