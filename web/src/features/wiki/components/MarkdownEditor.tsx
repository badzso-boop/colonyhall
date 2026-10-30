import { useTranslation } from "react-i18next";
import { Markdown, Tab, TabList, TabPanel, Tabs, Textarea } from "@/shared/ui";

/** A markdown textarea with a preview tab (new topic, edit, reply). */
export function MarkdownEditor({ value, onChange, label, rows = 8, placeholder }: { value: string; onChange: (v: string) => void; label: string; rows?: number; placeholder?: string }) {
  const { t } = useTranslation("wiki");
  return (
    <Tabs defaultValue="write" className="flex flex-col gap-1.5">
      <TabList label={label}>
        <Tab value="write">{t("write")}</Tab>
        <Tab value="preview">{t("preview")}</Tab>
      </TabList>
      <TabPanel value="write">
        <Textarea aria-label={label} rows={rows} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} className="font-mono text-ui" />
      </TabPanel>
      <TabPanel value="preview" className="min-h-24 rounded-md border border-border p-3">
        {value.trim() ? <Markdown text={value} copyLabel={t("copy")} copiedLabel={t("copied")} /> : <p className="text-meta text-fg-subtle">{t("nothing")}</p>}
      </TabPanel>
    </Tabs>
  );
}
