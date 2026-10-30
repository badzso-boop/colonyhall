import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";
import { Bot, Hash, MessageSquare, Palette } from "lucide-react";
import { cn, fuzzyScore } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { Dialog, Input } from "@/shared/ui";
import { useConversationOrder } from "@/features/conversation";
import { useActors } from "@/features/identity";
import { openOverlay, useAppOverlay } from "./overlays";

type Item = { id: string; group: "conversations" | "agents" | "pages"; label: string; icon: typeof Hash; run: () => void };

/** Ctrl K: jump to a conversation, an agent (its overview) or a page. Message search comes later. */
export function CommandPalette() {
  const { t } = useTranslation("layout");
  const open = useAppOverlay((s) => s.open === "palette");
  const navigate = useNavigate();
  const [, dispatch] = usePanes();
  const conversations = useConversationOrder();
  const actors = useActors();
  const [q, setQ] = useState("");
  const [pick, setPick] = useState(0);

  const items = useMemo(() => {
    const all: Item[] = [
      ...conversations.map((r) => ({
        id: `c-${r.channel.id}`, group: "conversations" as const, label: r.kind === "channel" ? `#${r.title}` : r.title,
        icon: r.kind === "channel" ? Hash : MessageSquare, run: () => navigate(`/c/${r.channel.id}`),
      })),
      ...[...(actors.data?.values() ?? [])].filter((a) => a.kind === "agent").map((a) => ({
        id: `a-${a.id}`, group: "agents" as const, label: `@${a.handle}`, icon: Bot,
        run: () => dispatch({ type: "openWork", tab: { kind: "agent", agentId: a.id, ref: "" } }),
      })),
      { id: "p-design", group: "pages" as const, label: t("pages.design"), icon: Palette, run: () => navigate("/design") },
    ];
    const query = q.trim();
    if (!query) return all.slice(0, 30);
    return all
      .map((i) => ({ i, s: fuzzyScore(i.label.replace(/^[#@]/, ""), query) }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s)
      .slice(0, 30)
      .map((x) => x.i);
  }, [conversations, actors.data, q, navigate, dispatch, t]);

  const close = () => {
    openOverlay(null);
    setQ("");
    setPick(0);
  };
  const run = (i: Item | undefined) => {
    if (!i) return;
    close();
    i.run();
  };

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? openOverlay("palette") : close())} title={t("palette.title")} closeLabel={t("common:close")} size="lg">
      <Input
        autoFocus
        role="combobox"
        aria-expanded
        aria-controls="palette-list"
        aria-label={t("palette.placeholder")}
        placeholder={t("palette.placeholder")}
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setPick(0);
        }}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            setPick((p) => (items.length ? (p + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length : 0));
          }
          if (e.key === "Enter") {
            e.preventDefault();
            run(items[pick]);
          }
        }}
      />
      <ul id="palette-list" role="listbox" aria-label={t("palette.title")} className="mt-2 max-h-80 overflow-y-auto">
        {items.length === 0 ? <li className="p-2 text-ui text-fg-muted">{t("palette.none")}</li> : null}
        {items.map((it, idx) => {
          const Icon = it.icon;
          return (
            <li
              key={it.id}
              role="option"
              aria-selected={idx === pick}
              onMouseDown={(e) => {
                e.preventDefault();
                run(it);
              }}
              className={cn("flex h-9 cursor-pointer items-center gap-2 rounded-md px-2 text-ui", idx === pick && "bg-surface-sunken")}
            >
              <Icon aria-hidden className="size-4 text-fg-subtle" />
              <span className="flex-1 truncate">{it.label}</span>
              <span className="text-meta text-fg-subtle">{t(`palette.${it.group}`)}</span>
            </li>
          );
        })}
      </ul>
    </Dialog>
  );
}
