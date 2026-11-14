// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/shared/lib";
import { Button, Dialog, Input, toast } from "@/shared/ui";
import { useCreateSection, useDeleteSection, useMoveConversation, useRenameSection } from "../api";
import type { Section } from "../model";
import { validSectionName } from "../sections";

/** Creates a section, or renames one when `section` is given. */
export function SectionNameDialog({ section, onClose }: { section?: Section; onClose: () => void }) {
  const { t } = useTranslation("conversation");
  const [name, setName] = useState(section?.name ?? "");
  const create = useCreateSection();
  const rename = useRenameSection();
  const valid = validSectionName(name);
  const submit = (e?: FormEvent) => {
    e?.preventDefault();
    if (!valid) return;
    const opts = { onSuccess: onClose, onError: () => toast(t("sections.saveFailed"), { tone: "danger" }) };
    if (section) rename.mutate({ id: section.id, name }, opts);
    else create.mutate(name, opts);
  };
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t(section ? "sections.renameTitle" : "sections.new")}
      closeLabel={t("new.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("new.cancel")}
          </Button>
          <Button variant="primary" disabled={!valid} loading={create.isPending || rename.isPending} onClick={() => submit()}>
            {t(section ? "sections.save" : "new.create")}
          </Button>
        </>
      }
    >
      <form onSubmit={submit} className="flex flex-col gap-1">
        <label className="flex flex-col gap-1">
          <span className="text-meta font-semibold text-fg-muted">{t("new.name")}</span>
          <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} aria-invalid={name.trim() !== "" && !valid} />
        </label>
        <p className="text-meta text-fg-subtle">{t("sections.nameHint")}</p>
      </form>
    </Dialog>
  );
}

/** Confirms deleting a section: its conversations go back to their default groups. */
export function DeleteSectionDialog({ section, onClose }: { section: Section; onClose: () => void }) {
  const { t } = useTranslation("conversation");
  const del = useDeleteSection();
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("sections.deleteTitle")}
      closeLabel={t("new.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("new.cancel")}
          </Button>
          <Button
            variant="danger"
            loading={del.isPending}
            onClick={() => del.mutate(section.id, { onSuccess: onClose, onError: () => toast(t("sections.saveFailed"), { tone: "danger" }) })}
          >
            {t("sections.delete")}
          </Button>
        </>
      }
    >
      <p className="text-ui">{t("sections.deleteBody", { name: section.name })}</p>
    </Dialog>
  );
}

const NEW = "__new__";
const DEFAULT = "__default__";

/**
 * "Áthelyezés…": the keyboard and phone alternative of dragging a row
 * (round 34, part 2). A section, the default group, or a new section.
 */
export function MoveDialog({ channelId, title, sections, from, onClose }: { channelId: string; title: string; sections: Section[]; from: string | null; onClose: () => void }) {
  const { t } = useTranslation("conversation");
  const [choice, setChoice] = useState<string>(from ?? DEFAULT);
  const [name, setName] = useState("");
  const move = useMoveConversation();
  const create = useCreateSection();
  const failed = () => toast(t("sections.failed"), { tone: "danger" });
  const ordered = [...sections].sort((a, b) => a.position - b.position);
  const ready = choice === NEW ? validSectionName(name) : choice !== (from ?? DEFAULT);
  const confirm = () => {
    if (choice === NEW) {
      create.mutate(name, {
        onSuccess: (s) => {
          move.mutate({ channelId, from, target: s.id }, { onError: failed });
          onClose();
        },
        onError: () => toast(t("sections.saveFailed"), { tone: "danger" }),
      });
      return;
    }
    move.mutate({ channelId, from, target: choice === DEFAULT ? null : choice }, { onError: failed });
    onClose();
  };
  const option = (value: string, label: string) => (
    <label key={value} className={cn("flex h-9 cursor-pointer items-center gap-2 rounded-md border px-2.5 text-ui", choice === value ? "border-accent bg-accent-soft" : "border-border")}>
      <input type="radio" name="move-target" value={value} checked={choice === value} onChange={() => setChoice(value)} className="accent-accent" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
    </label>
  );
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("rowMenu.moveTitle")}
      description={t("rowMenu.moveBody", { name: title })}
      closeLabel={t("new.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("new.cancel")}
          </Button>
          <Button variant="primary" disabled={!ready} loading={create.isPending} onClick={confirm}>
            {t("rowMenu.moveConfirm")}
          </Button>
        </>
      }
    >
      <fieldset className="flex flex-col gap-1.5">
        <legend className="sr-only">{t("rowMenu.moveTitle")}</legend>
        {ordered.map((s) => option(s.id, s.name))}
        {option(DEFAULT, t("rowMenu.defaultGroup"))}
        {option(NEW, t("rowMenu.newSection"))}
        {choice === NEW ? (
          <Input autoFocus aria-label={t("new.name")} value={name} onChange={(e) => setName(e.target.value)} placeholder={t("new.name")} />
        ) : null}
      </fieldset>
    </Dialog>
  );
}
