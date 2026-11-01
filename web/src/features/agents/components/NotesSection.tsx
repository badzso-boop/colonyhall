import { useTranslation } from "react-i18next";
import { useNow } from "@/shared/hooks";
import { localeOf } from "@/shared/i18n";
import { relativeTime } from "@/shared/lib";
import { usePanes } from "@/shared/panes";
import { openWindow } from "@/shared/windows";
import { Button, Skeleton } from "@/shared/ui";
import { useAgentInstructions } from "../api";
import { firstLines, type AgentOverview, type SettingsTab } from "../model";
import { Section } from "./Section";

/** 8. Jegyzetek: the three latest notebook entries; the Notes window follows the opened thread. */
export function NotesSection({ overview }: { overview: AgentOverview }) {
  const { t, i18n } = useTranslation("agents");
  const locale = localeOf(i18n.language);
  const now = useNow();
  const [, dispatch] = usePanes();
  const notes = [...(overview.collaboration?.notes ?? [])].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, 3);
  const open = (thread?: string) => {
    if (thread) dispatch({ type: "openThread", rootId: thread });
    openWindow("notes");
  };
  return (
    <Section id="notes" title={t("overview.notes.title")}>
      {notes.length ? (
        <ul className="flex flex-col gap-2">
          {notes.map((n) => (
            <li key={n.id}>
              <button type="button" onClick={() => open(n.thread_root_id)} className="flex w-full flex-col gap-0.5 rounded-md p-1 text-left hover:bg-surface-sunken">
                <span className="line-clamp-2 whitespace-pre-line">{n.text}</span>
                <span className="text-meta text-fg-subtle">{relativeTime(n.created_at, now, locale)}</span>
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-fg-muted">{t("overview.notes.empty")}</p>
      )}
      <Button size="sm" variant="ghost" className="mt-2" onClick={() => open(notes[0]?.thread_root_id)}>
        {t("overview.notes.open")}
      </Button>
    </Section>
  );
}

/** 9. Utasítás: the first lines of the agent's own instructions; the full text is in the settings dialog. */
export function InstructionsSection({ agentId, openSettings }: { agentId: string; openSettings: (tab: SettingsTab) => void }) {
  const { t } = useTranslation("agents");
  const ins = useAgentInstructions(agentId, true);
  const preview = ins.data ? firstLines(ins.data.text, 4) : null;
  return (
    <Section
      id="instructions"
      title={t("overview.instructions.title")}
      aside={
        <button type="button" onClick={() => openSettings("utasitas")} className="shrink-0 text-meta text-accent-text hover:underline">
          {t("overview.instructions.view")}
        </button>
      }
    >
      {!preview ? (
        ins.isError ? (
          <p className="text-fg-muted">{t("overview.instructions.empty")}</p>
        ) : (
          <Skeleton className="h-4 w-2/3" />
        )
      ) : preview.lines.length ? (
        <p className="font-mono text-meta whitespace-pre-wrap text-fg-muted">
          {preview.lines.join("\n")}
          {preview.more ? "\n…" : ""}
        </p>
      ) : (
        <p className="text-fg-muted">{t("overview.instructions.empty")}</p>
      )}
    </Section>
  );
}
