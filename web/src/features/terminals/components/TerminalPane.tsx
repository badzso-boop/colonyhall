import { useTranslation } from "react-i18next";
import { SquareTerminal } from "lucide-react";
import { EmptyState } from "@/shared/ui";

/** Placeholder until round 31 part 6 builds the terminal pane. */
export function TerminalPane({ agentId, name }: { agentId: string; name: string }) {
  const { t } = useTranslation("terminals");
  return <EmptyState icon={<SquareTerminal className="size-8" />} title={t("title")} body={`${agentId} ${name}`} />;
}
