import { useTranslation } from "react-i18next";
import { FolderOpen } from "lucide-react";
import { EmptyState } from "@/shared/ui";

/** Placeholder until round 31 part 7 builds the file browser pane. */
export function FileBrowserPane({ agentId, path }: { agentId: string; path: string }) {
  const { t } = useTranslation("files");
  return <EmptyState icon={<FolderOpen className="size-8" />} title={t("title")} body={`${agentId} ${path}`} />;
}
