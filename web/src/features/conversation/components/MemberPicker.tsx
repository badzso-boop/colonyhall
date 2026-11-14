// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Search } from "lucide-react";
import { cn, fuzzyScore, hueVar } from "@/shared/lib";
import { Avatar, Badge, Button, Checkbox, Dialog, Input, Select } from "@/shared/ui";
import type { Actor } from "../model";

/** Humans and agents (not archived) matching the query, humans first; the viewer is left out. */
export function pickable(actors: Actor[], query: string, exclude: string[] = []): Actor[] {
  const q = query.trim().toLowerCase();
  return actors
    .filter((a) => !a.archived_at && !exclude.includes(a.id))
    .map((a) => ({ a, score: q ? Math.max(fuzzyScore(a.handle, q), fuzzyScore(a.display_name, q)) : 1 }))
    .filter((x) => x.score > 0)
    .sort((x, y) => Number(x.a.kind === "agent") - Number(y.a.kind === "agent") || y.score - x.score || x.a.handle.localeCompare(y.a.handle))
    .map((x) => x.a);
}

/** The searchable checklist of people and agents; current members show disabled as "már tag". */
export function MemberChecklist({ actors, selected, onToggle, members = [], exclude = [] }: {
  actors: Actor[];
  selected: string[];
  onToggle: (id: string) => void;
  members?: string[];
  exclude?: string[];
}) {
  const { t } = useTranslation("conversation");
  const [query, setQuery] = useState("");
  const list = useMemo(() => pickable(actors, query, exclude), [actors, query, exclude]);
  return (
    <div className="flex flex-col gap-2">
      <label className="relative">
        <Search aria-hidden className="pointer-events-none absolute top-2 left-2 size-4 text-fg-subtle" />
        <Input type="search" aria-label={t("new.search")} placeholder={t("new.search")} value={query} onChange={(e) => setQuery(e.target.value)} className="pl-8" />
      </label>
      <ul aria-label={t("channel.members.candidates")} className="flex max-h-72 flex-col gap-px overflow-y-auto">
        {list.map((a) => {
          const member = members.includes(a.id);
          const id = `pick-${a.id}`;
          return (
            <li key={a.id}>
              <label htmlFor={id} className={cn("flex items-center gap-2 rounded-md px-2 py-1.5", member ? "opacity-60" : "cursor-pointer hover:bg-surface-sunken")}>
                <Checkbox id={id} checked={member || selected.includes(a.id)} disabled={member} onCheckedChange={() => onToggle(a.id)} />
                <Avatar kind={a.kind === "agent" ? "agent" : "human"} name={a.display_name || a.handle} emoji={a.avatar_emoji} size="sm" ringColor={hueVar(a.effective_color)} />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{a.display_name || a.handle}</span>
                  <span className="block truncate font-mono text-meta text-fg-muted">@{a.handle}</span>
                </span>
                {a.kind === "agent" ? <Badge>{t("channel.members.agent")}</Badge> : null}
                {member ? <span className="text-meta text-fg-subtle">{t("channel.members.already")}</span> : null}
              </label>
            </li>
          );
        })}
        {list.length === 0 ? <li className="px-2 py-3 text-meta text-fg-subtle">{t("new.none")}</li> : null}
      </ul>
    </div>
  );
}

/** The member's access: "Írhat" or "Csak olvashat" (read-only: reads, never posts, reacts or uploads). */
export function AccessSelect({ canWrite, onChange, label, disabled }: { canWrite: boolean; onChange: (canWrite: boolean) => void; label: string; disabled?: boolean }) {
  const { t } = useTranslation("conversation");
  return (
    <Select aria-label={label} value={canWrite ? "write" : "read"} disabled={disabled} onChange={(e) => onChange(e.target.value === "write")} className="w-36">
      <option value="write">{t("channel.members.canWrite")}</option>
      <option value="read">{t("channel.members.readOnly")}</option>
    </Select>
  );
}

/** "Tagok hozzáadása…": pick several, add them with one batch request, all with the chosen access. */
export function MemberPicker({ actors, members, pending, onAdd, onClose }: {
  actors: Actor[];
  members: string[];
  pending: boolean;
  onAdd: (ids: string[], canWrite: boolean) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation("conversation");
  const [selected, setSelected] = useState<string[]>([]);
  const [canWrite, setCanWrite] = useState(true);
  const toggle = (id: string) => setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));
  return (
    <Dialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("channel.members.addTitle")}
      closeLabel={t("new.close")}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t("new.cancel")}
          </Button>
          <Button variant="primary" disabled={selected.length === 0} loading={pending} onClick={() => onAdd(selected, canWrite)}>
            {t("channel.members.addCount", { count: selected.length })}
          </Button>
        </>
      }
    >
      <MemberChecklist actors={actors} selected={selected} onToggle={toggle} members={members} />
      <label className="mt-3 flex items-center gap-2">
        <span className="flex-1 text-meta font-semibold text-fg-muted">{t("channel.members.access")}</span>
        <AccessSelect canWrite={canWrite} onChange={setCanWrite} label={t("channel.members.access")} />
      </label>
    </Dialog>
  );
}
