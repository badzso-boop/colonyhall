// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { SecretOnce } from "@/features/admin";
import { ApiError } from "@/shared/api";
import { fullTime } from "@/shared/lib";
import { Button, Dialog, Input } from "@/shared/ui";
import { useCreateCustomer } from "../api";
import { slugFromName, validSlug } from "../model";

/** A new customer: name and subdomain; then its owner invite, shown once. */
export function CreateCustomerDialog({ onClose }: { onClose: () => void }) {
  const { t, i18n } = useTranslation("operator");
  const create = useCreateCustomer();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const effectiveSlug = slugEdited ? slug : slugFromName(name);
  const slugBad = effectiveSlug !== "" && !validSlug(effectiveSlug);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!validSlug(effectiveSlug)) return;
    create.mutate({ name: name.trim(), slug: effectiveSlug });
  };
  const err = create.error;
  const error = err
    ? err instanceof ApiError && err.problem?.type === "slug_taken"
      ? t("create.slugTaken")
      : err instanceof ApiError && err.status === 400
        ? t("create.invalid")
        : err.message
    : null;

  if (create.data) {
    const { customer, invite } = create.data;
    return (
      <Dialog open onOpenChange={(o) => !o && onClose()} title={t("create.doneTitle", { name: customer.name })} closeLabel={t("common:close")}
        footer={<Button variant="primary" onClick={onClose}>{t("done")}</Button>}>
        <div className="flex flex-col gap-3 text-ui">
          <p>{t("invite.handOver", { expires: fullTime(invite.expires_at, i18n.language) })}</p>
          <SecretOnce secret={invite.url} />
        </div>
      </Dialog>
    );
  }
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()} title={t("create.title")} closeLabel={t("common:close")}
      footer={
        <>
          <Button onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" form="operator-create" variant="primary" loading={create.isPending} disabled={!name.trim() || !validSlug(effectiveSlug)}>
            {t("create.submit")}
          </Button>
        </>
      }>
      <form id="operator-create" onSubmit={submit} className="flex flex-col gap-3 text-ui">
        <label className="flex flex-col gap-1">
          <span className="font-medium">{t("create.name")}</span>
          <Input required maxLength={80} value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1">
          <span className="font-medium">{t("create.slug")}</span>
          <Input
            required
            autoCapitalize="none"
            spellCheck={false}
            value={effectiveSlug}
            aria-invalid={slugBad}
            onChange={(e) => {
              setSlugEdited(true);
              setSlug(e.target.value.toLowerCase());
            }}
            className="font-mono"
          />
          <span className={slugBad ? "text-meta text-danger" : "text-meta text-fg-subtle"}>{t("create.slugHelp")}</span>
        </label>
        {error ? (
          <p role="alert" className="text-danger">
            {error}
          </p>
        ) : null}
      </form>
    </Dialog>
  );
}
