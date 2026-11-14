// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert

import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDown, ArrowUp, FolderInput, MoreHorizontal, UserRound } from "lucide-react";
import { cn } from "@/shared/lib";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger, toast } from "@/shared/ui";
import { useMoveConversation } from "../api";
import type { Section, SidebarRow } from "../model";
import { itemOrder } from "../sections";
import { MuteMenuItems } from "./ChannelHeader";
import { MoveDialog } from "./SectionDialogs";

type Item = { key: string; label: string; icon: ReactNode; onSelect: () => void; danger?: boolean };

/**
 * The "…" menu of a sidebar conversation row (round 34, part 2). Every
 * item is listed in `items` below; muting comes from the channel header's
 * items. Dialogs an item opens are kept in `dialog`.
 */
export function RowMenu({ row, sections, sectionId, onShowProfile }: { row: SidebarRow; sections: Section[]; sectionId: string | null; onShowProfile?: () => void }) {
  const { t } = useTranslation("conversation");
  const [dialog, setDialog] = useState<"move" | null>(null);
  const move = useMoveConversation();
  const channelId = row.channel.id;
  const order = sectionId ? itemOrder(sections, sectionId) : [];
  const at = order.indexOf(channelId);
  const shift = (by: -1 | 1) =>
    move.mutate({ channelId, from: sectionId, target: sectionId, index: at + by }, { onError: () => toast(t("sections.failed"), { tone: "danger" }) });

  const items: Item[] = [
    ...(onShowProfile ? [{ key: "profile", label: t("rowMenu.profile"), icon: <UserRound aria-hidden className="size-4" />, onSelect: onShowProfile }] : []),
    { key: "move", label: t("rowMenu.move"), icon: <FolderInput aria-hidden className="size-4" />, onSelect: () => setDialog("move") },
    ...(sectionId && at > 0 ? [{ key: "up", label: t("sections.moveUp"), icon: <ArrowUp aria-hidden className="size-4" />, onSelect: () => shift(-1) }] : []),
    ...(sectionId && at >= 0 && at < order.length - 1
      ? [{ key: "down", label: t("sections.moveDown"), icon: <ArrowDown aria-hidden className="size-4" />, onSelect: () => shift(1) }]
      : []),
  ];

  return (
    <>
      <Menu>
        <MenuTrigger asChild>
          <button
            type="button"
            aria-label={t("rowMenu.label", { name: row.title })}
            title={t("rowMenu.label", { name: row.title })}
            className={cn(
              "absolute top-1 right-1 grid size-6 place-items-center rounded-sm bg-surface-sunken text-fg-subtle hover:text-fg",
              "opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100 pointer-coarse:opacity-100",
            )}
          >
            <MoreHorizontal aria-hidden className="size-4" />
          </button>
        </MenuTrigger>
        <MenuContent>
          {items.map((i) => (
            <MenuItem key={i.key} icon={i.icon} onSelect={i.onSelect} danger={i.danger}>
              {i.label}
            </MenuItem>
          ))}
          <MenuSeparator />
          <MuteMenuItems channel={row.channel} />
        </MenuContent>
      </Menu>
      {dialog === "move" ? <MoveDialog channelId={channelId} title={row.title} sections={sections} from={sectionId} onClose={() => setDialog(null)} /> : null}
    </>
  );
}
