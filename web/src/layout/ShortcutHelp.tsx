import { useState } from "react";
import { useTranslation } from "react-i18next";
import { keyLabels, useShortcuts, type Scope } from "@/shared/lib";
import { Dialog, Input, Kbd } from "@/shared/ui";
import { openOverlay, useAppOverlay } from "./overlays";

const order: Scope[] = ["global", "conversation", "composer", "pane", "page"];

/** "?" — every registered shortcut, grouped by scope, with a filter. */
export function ShortcutHelp() {
  const { t } = useTranslation();
  const open = useAppOverlay((s) => s.open === "help");
  const list = useShortcuts((s) => s.list);
  const [filter, setFilter] = useState("");
  const q = filter.trim().toLowerCase();
  // One row per key and scope, even if several components declared it.
  const seen = new Set<string>();
  const rows = list.filter((s) => {
    const k = `${s.scope}|${s.keys}`;
    if (seen.has(k)) return false;
    seen.add(k);
    return !q || t(`shortcuts.${s.description}`).toLowerCase().includes(q) || s.keys.includes(q);
  });
  return (
    <Dialog open={open} onOpenChange={(o) => openOverlay(o ? "help" : null)} title={t("shortcuts.title")} closeLabel={t("close")} size="lg">
      <Input aria-label={t("shortcuts.filter")} placeholder={t("shortcuts.filter")} value={filter} onChange={(e) => setFilter(e.target.value)} className="mb-3" />
      {order.map((scope) => {
        const items = rows.filter((r) => r.scope === scope);
        if (!items.length) return null;
        return (
          <section key={scope} className="mb-3">
            <h3 className="mb-1 text-meta font-semibold tracking-wide text-fg-subtle uppercase">{t(`shortcuts.scopes.${scope}`)}</h3>
            <dl className="divide-y divide-border">
              {items.map((s) => (
                <div key={s.id} className="flex items-center justify-between gap-3 py-1.5">
                  <dt className="text-ui">{t(`shortcuts.${s.description}`)}</dt>
                  <dd className="flex shrink-0 gap-1">
                    {keyLabels(s.keys).map((k) => (
                      <Kbd key={k}>{k}</Kbd>
                    ))}
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        );
      })}
    </Dialog>
  );
}
