import { useTranslation } from "react-i18next";
import { cn } from "@/shared/lib";
import { KINDS, dispatchWindow, useWindows } from "@/shared/windows";
import { windowMeta } from "./FloatingWindows";

/** The header's dock: every floating window, open (underlined), minimised or closed. */
export function WindowDock() {
  const { t } = useTranslation("layout");
  const wins = useWindows((s) => s.wins);
  return (
    <div role="group" aria-label={t("windows.dock")} className="hidden items-center gap-0.5 border-x border-border px-1 md:flex">
      {KINDS.map((k) => {
        const Icon = windowMeta[k].icon;
        const w = wins[k];
        const label = w.state === "min" ? t("windows.restore", { name: t(`windows.${k}`) }) : t("windows.popOut", { name: t(`windows.${k}`) });
        return (
          <button
            key={k}
            type="button"
            aria-label={label}
            aria-pressed={w.state !== "closed"}
            title={label}
            onClick={() => dispatchWindow({ type: w.state === "min" ? "restore" : "open", kind: k })}
            className={cn(
              "relative grid size-8 place-items-center rounded-md text-fg-muted hover:bg-surface-sunken hover:text-fg",
              w.state === "open" && "text-fg after:absolute after:bottom-1 after:left-1/2 after:h-0.5 after:w-3 after:-translate-x-1/2 after:rounded-full after:bg-accent",
            )}
          >
            <Icon aria-hidden className="size-4" />
          </button>
        );
      })}
    </div>
  );
}
